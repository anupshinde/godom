// bridge_v2.js — godom VDOM patch executor.
//
// Receives binary protobuf VDomMessage from Go over WebSocket.
// On "init": sets innerHTML, indexes DOM, registers events.
// On "patch": applies minimal DOM mutations from the diff algorithm.
//
// Structure:
//   1. State & globals
//   2. Connection — WebSocket with auto-reconnect and disconnect overlay
//   3. DOM indexing — gid map from data-gid attributes
//   4. Patch execution — apply patches by type
//   5. Facts application — properties, attributes, styles, events
//   6. Event registration — wire DOM events to send messages back to Go
//   7. Drag & drop — draggable/dropzone setup with group filtering
//   8. Plugin lifecycle — init/update for JS plugins

(function() {

    // =========================================================================
    // 1. State & globals
    // =========================================================================

    var ws;
    var gidMap = {};        // data-gid → DOM element
    var eventMap = {};      // "gid:event[:key]" → latest event config
    var pluginState = {};   // gid → true if plugin init called
    var rootNode;           // the root DOM node (document.body or a container)

    var Proto = godomProto;
    var textEncoder = new TextEncoder();
    var textDecoder = new TextDecoder();

    // =========================================================================
    // 2. Connection — WebSocket with auto-reconnect and disconnect overlay
    // =========================================================================

    var overlay = null;

    function showDisconnectOverlay(errorMsg) {
        if (overlay) return;
        overlay = document.createElement("div");
        overlay.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:rgba(0,0,0,0.7);backdrop-filter:blur(6px);-webkit-backdrop-filter:blur(6px);display:flex;align-items:center;justify-content:center;transition:opacity 0.3s";
        var title = errorMsg ? "Application Crashed" : "Disconnected";
        var subtitle = errorMsg ? "Restart the application to continue" : "Waiting for server\u2026";
        var html = '<div style="color:#fff;font-family:system-ui,sans-serif;text-align:center">'
            + '<div style="font-size:1.5rem;margin-bottom:0.5rem;color:#ff4d4d;font-weight:600">' + title + '</div>'
            + '<div style="font-size:1.05rem;color:#ccc">' + subtitle + '</div>';
        if (errorMsg) {
            var safe = errorMsg.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;");
            html += '<div style="margin-top:1.2rem;background:rgba(0,0,0,0.5);border:1px solid #444;border-radius:8px;padding:0.8rem 1.2rem;text-align:left;max-width:80vw;overflow-x:auto">'
                + '<pre style="margin:0;font-size:0.85rem;color:#ffaaaa;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;white-space:pre-wrap;word-break:break-word">' + safe + '</pre></div>';
        }
        html += '</div>';
        overlay.innerHTML = html;
        document.body.appendChild(overlay);
    }

    function hideDisconnectOverlay() {
        if (!overlay) return;
        overlay.remove();
        overlay = null;
    }

    var reconnectDelay = 1000;

    function connect() {
        var wsProto = location.protocol === "https:" ? "wss://" : "ws://";
        ws = new WebSocket(wsProto + location.host + "/ws");
        ws.binaryType = "arraybuffer";

        ws.onmessage = function(evt) {
            var msg = Proto.VDomMessage.decode(new Uint8Array(evt.data));
            if (msg.type === "init") {
                hideDisconnectOverlay();
                reconnectDelay = 1000;
                gidMap = {};
                eventMap = {};
                pluginState = {};
                // Set full HTML content
                document.body.innerHTML = textDecoder.decode(msg.html);
                rootNode = document.body;
                indexDOM(rootNode);
                registerEvents(msg.events);
                initPlugins(rootNode);
            } else if (msg.type === "patch") {
                applyPatches(msg.patches);
            }
        };

        ws.onclose = function(evt) {
            var errorMsg = evt.reason || null;
            showDisconnectOverlay(errorMsg);
            if (!errorMsg) {
                setTimeout(connect, reconnectDelay);
                reconnectDelay = Math.min(reconnectDelay * 2, 30000);
            }
        };

        ws.onerror = function() {
            ws.close();
        };
    }

    // =========================================================================
    // 3. DOM indexing — map data-gid attributes
    // =========================================================================

    function indexDOM(root) {
        var all = root.querySelectorAll("[data-gid]");
        for (var i = 0; i < all.length; i++) {
            gidMap[all[i].getAttribute("data-gid")] = all[i];
        }
    }

    function getEl(gid) {
        return gidMap[gid] || null;
    }

    // =========================================================================
    // 4. Patch execution — dispatch by op type
    // =========================================================================

    // Get a DOM node by its depth-first traversal index relative to rootNode.
    // Index 0 = rootNode itself.
    function getNodeByIndex(index) {
        if (index === 0) return rootNode;
        var count = 0;
        return walkDFS(rootNode, index, {count: 0});
    }

    // Depth-first walk to find node at given index.
    function walkDFS(node, target, state) {
        for (var i = 0; i < node.childNodes.length; i++) {
            var child = node.childNodes[i];
            state.count++;
            if (state.count === target) return child;
            if (child.childNodes && child.childNodes.length > 0) {
                var found = walkDFS(child, target, state);
                if (found) return found;
            }
        }
        return null;
    }

    function applyPatches(patches) {
        if (!patches) return;
        // Resolve all target DOM nodes BEFORE applying any patches.
        // Structural patches (append, redraw, remove) change the DOM tree,
        // which invalidates DFS indices for subsequent patches. By collecting
        // all target nodes upfront from the unmodified DOM, every patch
        // operates on the correct node.
        var resolved = [];
        for (var i = 0; i < patches.length; i++) {
            resolved.push(getNodeByIndex(patches[i].index));
        }
        for (var i = 0; i < patches.length; i++) {
            applyPatch(resolved[i], patches[i]);
        }
    }

    function applyPatch(node, patch) {
        if (!node) return;

        switch (patch.op) {
            case "redraw":
                execRedraw(node, patch);
                break;
            case "text":
                execText(node, patch);
                break;
            case "facts":
                execFacts(node, patch);
                break;
            case "append":
                execAppend(node, patch);
                break;
            case "remove-last":
                execRemoveLast(node, patch);
                break;
            case "reorder":
                execReorder(node, patch);
                break;
            case "plugin":
                execPlugin(node, patch);
                break;
            case "lazy":
                // Lazy patches contain sub-patches — apply them recursively.
                applyPatches(patch.subPatches);
                break;
        }
    }

    // --- Redraw: replace entire node with new HTML ---
    function execRedraw(node, patch) {
        var html = textDecoder.decode(patch.htmlContent);
        var tmp = document.createElement("div");
        tmp.innerHTML = html;

        // Replace old node with new content
        var newNode = tmp.firstChild;
        if (newNode) {
            // Clean up old node from caches
            cleanNode(node);
            node.parentNode.replaceChild(newNode, node);
            // Index new node and its children
            if (newNode.nodeType === 1) {
                if (newNode.hasAttribute("data-gid")) {
                    gidMap[newNode.getAttribute("data-gid")] = newNode;
                }
                indexDOM(newNode);
            }
        }

        // Register events and init plugins on the new nodes
        registerEvents(patch.patchEvents);
        if (newNode && newNode.nodeType === 1) initPlugins(newNode);
    }

    // --- Text: update text content ---
    function execText(node, patch) {
        if (node.nodeType === 3) {
            // Text node
            node.nodeValue = patch.text;
        } else {
            // Element node — set textContent
            node.textContent = patch.text;
        }
    }

    // --- Facts: apply property/attribute/style/event changes ---
    function execFacts(node, patch) {
        if (!patch.facts || !patch.facts.length) return;
        var diff = JSON.parse(textDecoder.decode(patch.facts));
        applyFactsDiff(node, diff);
    }

    // --- Append: add children at end ---
    function execAppend(node, patch) {
        var html = textDecoder.decode(patch.htmlContent);
        // Use contextual container for correct parsing
        var tmp = createContextContainer(node);
        tmp.innerHTML = html;
        while (tmp.firstChild) {
            var child = tmp.firstChild;
            node.appendChild(child);
            if (child.nodeType === 1) {
                if (child.hasAttribute("data-gid")) {
                    gidMap[child.getAttribute("data-gid")] = child;
                }
                indexDOM(child);
            }
        }
        registerEvents(patch.patchEvents);
    }

    // --- RemoveLast: remove N children from end ---
    function execRemoveLast(node, patch) {
        var count = patch.count;
        for (var i = 0; i < count; i++) {
            if (node.lastChild) {
                cleanNode(node.lastChild);
                node.removeChild(node.lastChild);
            }
        }
    }

    // --- Reorder: keyed children insert/remove/move ---
    function execReorder(node, patch) {
        if (!patch.reorder || !patch.reorder.length) return;
        var data = JSON.parse(textDecoder.decode(patch.reorder));

        // Apply removes first (in reverse order to preserve indices)
        if (data.rem) {
            for (var i = data.rem.length - 1; i >= 0; i--) {
                var rem = data.rem[i];
                var child = node.childNodes[rem.i];
                if (child) {
                    cleanNode(child);
                    node.removeChild(child);
                }
            }
        }

        // Apply inserts
        if (data.ins) {
            for (var j = 0; j < data.ins.length; j++) {
                var ins = data.ins[j];
                var newChild;
                if (ins.h) {
                    var tmp = createContextContainer(node);
                    tmp.innerHTML = ins.h;
                    newChild = tmp.firstChild;
                }
                if (newChild) {
                    var ref = node.childNodes[ins.i] || null;
                    node.insertBefore(newChild, ref);
                    if (newChild.nodeType === 1) {
                        if (newChild.hasAttribute("data-gid")) {
                            gidMap[newChild.getAttribute("data-gid")] = newChild;
                        }
                        indexDOM(newChild);
                    }
                }
            }
        }

        // Apply sub-patches for changed children
        if (patch.subPatches) {
            applyPatches(patch.subPatches);
        }
    }

    // --- Plugin: forward data to JS plugin handler ---
    function execPlugin(node, patch) {
        if (!patch.pluginData || !patch.pluginData.length) return;
        var data = JSON.parse(textDecoder.decode(patch.pluginData));
        var gid = node.getAttribute && node.getAttribute("data-gid");
        var pluginName = node.getAttribute && node.getAttribute("data-g-plugin");
        var handler = window.godom && window.godom._plugins && window.godom._plugins[pluginName];
        if (!handler) return;

        if (gid && !pluginState[gid]) {
            handler.init(node, data);
            pluginState[gid] = true;
        } else {
            handler.update(node, data);
        }
    }

    // --- Plugin init: scan for data-g-plugin-init elements on initial render ---
    function initPlugins(root) {
        var els = root.querySelectorAll("[data-g-plugin-init]");
        for (var i = 0; i < els.length; i++) {
            var el = els[i];
            var pluginName = el.getAttribute("data-g-plugin");
            var handler = window.godom && window.godom._plugins && window.godom._plugins[pluginName];
            if (!handler) continue;
            var gid = el.getAttribute("data-gid");
            if (!gid) continue;
            try {
                var data = JSON.parse(el.getAttribute("data-g-plugin-init"));
                handler.init(el, data);
                pluginState[gid] = true;
            } catch(e) {}
            // Remove the init data attribute — no longer needed
            el.removeAttribute("data-g-plugin-init");
        }
    }

    // =========================================================================
    // 5. Facts application — properties, attributes, styles, events
    // =========================================================================

    function applyFactsDiff(el, diff) {
        // Properties
        if (diff.p) {
            for (var key in diff.p) {
                var val = diff.p[key];
                if (val === null || val === undefined) {
                    // Remove property
                    el[key] = "";
                } else {
                    el[key] = val;
                }
            }
        }

        // Attributes
        if (diff.a) {
            for (var key in diff.a) {
                var val = diff.a[key];
                if (val === "") {
                    el.removeAttribute(key);
                } else {
                    el.setAttribute(key, val);
                }
            }
        }

        // Namespaced attributes
        if (diff.an) {
            for (var key in diff.an) {
                var nsAttr = diff.an[key];
                if (!nsAttr || (!nsAttr.ns && !nsAttr.v)) {
                    el.removeAttributeNS(null, key);
                } else {
                    el.setAttributeNS(nsAttr.ns, key, nsAttr.v);
                }
            }
        }

        // Styles
        if (diff.s) {
            for (var key in diff.s) {
                var val = diff.s[key];
                if (val === "") {
                    el.style.removeProperty(key);
                } else {
                    el.style.setProperty(key, val);
                }
            }
        }

        // Events
        if (diff.e) {
            for (var key in diff.e) {
                var evtData = diff.e[key];
                if (evtData === null) {
                    // Remove event — clear from eventMap
                    // (DOM listener stays but becomes a no-op since eventMap entry is gone)
                    var gid = el.getAttribute("data-gid");
                    if (gid) delete eventMap[gid + ":" + key];
                } else {
                    // Add/update event
                    registerEvents([evtData]);
                }
            }
        }
    }

    // =========================================================================
    // 6. Event registration — wire DOM events to send messages back to Go
    // =========================================================================

    function sendEnvelope(msg, args, value) {
        var env = {msg: msg};
        if (args) env.args = args;
        if (value) env.value = value;
        ws.send(Proto.Envelope.encode(env).finish());
    }

    function registerEvents(evts) {
        if (!evts) return;
        for (var i = 0; i < evts.length; i++) {
            var e = evts[i];
            var el = getEl(e.gid);
            if (!el) continue;

            var key = e.gid + ":" + e.event;
            if (e.event === "keydown" && e.key) {
                key += ":" + e.key;
            }
            eventMap[key] = e;

            // Skip if listener already attached for this event type
            if (el.getAttribute("data-evt-" + e.event)) continue;
            el.setAttribute("data-evt-" + e.event, "1");

            if (e.event === "input") {
                (function(k, elem) {
                    elem.addEventListener("input", function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        var valBytes = textEncoder.encode(JSON.stringify(elem.value));
                        sendEnvelope(ev.msg, null, valBytes);
                    });
                })(key, el);

            } else if (e.event === "keydown") {
                (function(gid, elem) {
                    elem.addEventListener("keydown", function(ke) {
                        var ev = eventMap[gid + ":keydown:" + ke.key]
                              || eventMap[gid + ":keydown"];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(e.gid, el);

            } else if (e.event === "mousedown" || e.event === "mouseup") {
                (function(k, elem, evtName) {
                    elem.addEventListener(evtName, function(me) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [me.offsetX, me.offsetY]);
                    });
                })(key, el, e.event);

            } else if (e.event === "mousemove") {
                (function(k, elem) {
                    var pending = null;
                    var scheduled = false;
                    elem.addEventListener("mousemove", function(me) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        pending = me;
                        if (scheduled) return;
                        scheduled = true;
                        requestAnimationFrame(function() {
                            sendEnvelope(ev.msg, [pending.offsetX, pending.offsetY]);
                            pending = null;
                            scheduled = false;
                        });
                    });
                })(key, el);

            } else if (e.event === "wheel") {
                (function(k, elem) {
                    elem.addEventListener("wheel", function(we) {
                        we.preventDefault();
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [we.deltaY]);
                    }, {passive: false});
                })(key, el);

            } else if (e.event === "drop") {
                setupDropHandler(key, el, e.key || "");

            } else {
                (function(k, elem, evtName) {
                    elem.addEventListener(evtName, function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(key, el, e.event);
            }
        }
    }

    // =========================================================================
    // 7. Drag & drop
    // =========================================================================

    function setupDraggable(el, gid, group, value) {
        el.setAttribute("draggable", "true");
        el.dataset.gDrag = value || "";
        el.dataset.gDragGroup = group || "";
        if (!el.getAttribute("data-g-dragstart")) {
            el.setAttribute("data-g-dragstart", "1");
            el.addEventListener("dragstart", function(de) {
                de.dataTransfer.effectAllowed = "move";
                var dragEl = de.target.closest("[data-g-drag]");
                var grp = dragEl.dataset.gDragGroup || "";
                de.dataTransfer.setData("application/x-godom-" + grp, dragEl.dataset.gDrag);
                dragEl.classList.add("g-dragging");
            });
            el.addEventListener("dragend", function(de) {
                de.target.closest("[data-g-drag]").classList.remove("g-dragging");
                var overs = document.querySelectorAll(".g-drag-over,.g-drag-over-above,.g-drag-over-below");
                for (var j = 0; j < overs.length; j++) {
                    overs[j].classList.remove("g-drag-over", "g-drag-over-above", "g-drag-over-below");
                }
            });
        }
    }

    function setupDropHandler(key, el, group) {
        var mimeType = "application/x-godom-" + group;
        var dragCounter = 0;

        function hasMatch(dt) {
            for (var t = 0; t < dt.types.length; t++) {
                if (dt.types[t] === mimeType) return true;
            }
            return false;
        }

        el.addEventListener("dragover", function(de) {
            if (!hasMatch(de.dataTransfer)) return;
            de.preventDefault();
            de.dataTransfer.dropEffect = "move";
            var rect = el.getBoundingClientRect();
            var isAbove = de.clientY < rect.top + rect.height / 2;
            el.classList.remove("g-drag-over-above", "g-drag-over-below");
            el.classList.add("g-drag-over");
            el.classList.add(isAbove ? "g-drag-over-above" : "g-drag-over-below");
        });

        el.addEventListener("dragenter", function(de) {
            if (!hasMatch(de.dataTransfer)) return;
            de.preventDefault();
            dragCounter++;
        });
        el.addEventListener("dragleave", function() {
            if (dragCounter === 0) return;
            dragCounter--;
            if (dragCounter === 0) {
                el.classList.remove("g-drag-over", "g-drag-over-above", "g-drag-over-below");
            }
        });

        el.addEventListener("drop", function(de) {
            dragCounter = 0;
            el.classList.remove("g-drag-over", "g-drag-over-above", "g-drag-over-below");
            if (!hasMatch(de.dataTransfer)) return;
            de.preventDefault();
            var ev = eventMap[key];
            if (!ev) return;
            var fromStr = de.dataTransfer.getData(mimeType);
            var toStr = el.dataset.gDrop || el.dataset.gDrag || "";
            var rect = el.getBoundingClientRect();
            var position = de.clientY < rect.top + rect.height / 2 ? "above" : "below";
            function smartVal(s) { var n = Number(s); return s !== "" && !isNaN(n) ? n : s; }
            var dropArgs = [smartVal(fromStr), smartVal(toStr), position];
            sendEnvelope(ev.msg, null, textEncoder.encode(JSON.stringify(dropArgs)));
        });
    }

    // =========================================================================
    // 8. Helpers
    // =========================================================================

    // Create a container element with the correct context for HTML parsing.
    // Handles browser quirks where certain elements (tr, td, option) can only
    // be children of specific parent elements.
    function createContextContainer(parentEl) {
        var tag = parentEl.tagName;
        if (!tag) return document.createElement("div");
        tag = tag.toUpperCase();
        switch (tag) {
            case "TABLE": case "THEAD": case "TBODY": case "TFOOT":
                return document.createElement("tbody");
            case "TR":
                return document.createElement("tr");
            case "SELECT": case "OPTGROUP":
                return document.createElement("select");
            default:
                return document.createElement("div");
        }
    }

    // Remove a DOM node and clean up all cached references.
    function cleanNode(node) {
        if (!node) return;
        if (node.nodeType === 1) {
            var gid = node.getAttribute("data-gid");
            if (gid) {
                delete gidMap[gid];
                delete pluginState[gid];
                // Clean eventMap entries for this gid
                for (var key in eventMap) {
                    if (key.indexOf(gid + ":") === 0) {
                        delete eventMap[key];
                    }
                }
            }
            // Recurse into children
            for (var i = 0; i < node.childNodes.length; i++) {
                cleanNode(node.childNodes[i]);
            }
        }
    }

    // =========================================================================
    // 9. Plugin registration (global API)
    // =========================================================================

    // Plugins register themselves via godom.register(name, {init, update}).
    // This must be available before bridge connects.
    if (!window.godom) window.godom = {};
    if (!window.godom._plugins) window.godom._plugins = {};
    window.godom.register = function(name, handler) {
        window.godom._plugins[name] = handler;
    };

    // =========================================================================
    // Boot
    // =========================================================================

    connect();
})();
