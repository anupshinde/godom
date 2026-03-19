// bridge.js — godom VDOM bridge.
//
// Receives binary protobuf VDomMessage from Go over WebSocket.
// On "init": builds DOM from tree description, registers events.
// On "patch": applies minimal DOM mutations using nodeMap[id] lookups.
//
// Structure:
//   1. State & globals
//   2. Connection — WebSocket with auto-reconnect and disconnect overlay
//   3. DOM construction — build DOM from tree descriptions
//   4. Patch execution — apply patches by type
//   5. Facts application — properties, attributes, styles, events
//   6. Event handling — wire DOM events to send messages back to Go
//   7. Drag & drop — draggable/dropzone setup with group filtering
//   8. Plugin lifecycle — init/update for JS plugins
//   9. Helpers

(function() {

    // =========================================================================
    // 1. State & globals
    // =========================================================================

    var ws;
    var nodeMap = {};       // node ID (int) → DOM node
    var eventMap = {};      // "nodeId:event[:key]" → latest event config
    var pluginState = {};   // node ID → true if plugin init called
    var rootNode;           // the root DOM node (document.body)

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
                nodeMap = {};
                eventMap = {};
                pluginState = {};
                // Build DOM from tree description
                var tree = JSON.parse(textDecoder.decode(msg.tree));
                document.body.innerHTML = "";
                rootNode = document.body;
                if (tree) {
                    var domNode = buildDOM(tree);
                    if (domNode) {
                        // If the tree root is <body>, use its children
                        if (tree.tag === "body") {
                            while (domNode.firstChild) {
                                rootNode.appendChild(domNode.firstChild);
                            }
                            // Map body's ID to rootNode
                            nodeMap[tree.id] = rootNode;
                        } else {
                            rootNode.appendChild(domNode);
                        }
                    }
                }
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
    // 3. DOM construction — build DOM nodes from tree descriptions
    // =========================================================================

    // Build a DOM node from a wire tree description and register it in nodeMap.
    function buildDOM(tree) {
        if (!tree) return null;

        if (tree.t === "text") {
            var textNode = document.createTextNode(tree.x || "");
            if (tree.id) nodeMap[tree.id] = textNode;
            return textNode;
        }

        // Element node (including keyed)
        var el;
        if (tree.ns) {
            el = document.createElementNS(tree.ns, tree.tag);
        } else {
            el = document.createElement(tree.tag);
        }

        if (tree.id) nodeMap[tree.id] = el;

        // Apply facts: props, attrs, styles, namespaced attrs
        applyProps(el, tree.p);
        applyAttrs(el, tree.a);
        applyAttrsNS(el, tree.an);
        applyStyles(el, tree.s);

        // Register events
        if (tree.ev) {
            registerNodeEvents(tree.id, el, tree.ev);
        }

        // Build children
        if (tree.c) {
            for (var i = 0; i < tree.c.length; i++) {
                var child = buildDOM(tree.c[i]);
                if (child) el.appendChild(child);
            }
        }

        // Plugin init
        if (tree.plug) {
            var handler = window.godom && window.godom._plugins && window.godom._plugins[tree.plug];
            if (handler && tree.pd !== undefined) {
                handler.init(el, tree.pd);
                pluginState[tree.id] = true;
            }
        }

        return el;
    }

    function applyProps(el, props) {
        if (!props) return;
        for (var key in props) {
            el[key] = props[key];
        }
    }

    function applyAttrs(el, attrs) {
        if (!attrs) return;
        for (var key in attrs) {
            el.setAttribute(key, attrs[key]);
        }
    }

    function applyAttrsNS(el, attrsNS) {
        if (!attrsNS) return;
        for (var key in attrsNS) {
            el.setAttributeNS(attrsNS[key].ns, key, attrsNS[key].v);
        }
    }

    function applyStyles(el, styles) {
        if (!styles) return;
        for (var key in styles) {
            el.style.setProperty(key, styles[key]);
        }
    }

    // =========================================================================
    // 4. Patch execution — dispatch by op type
    // =========================================================================

    function applyPatches(patches) {
        if (!patches) return;
        for (var i = 0; i < patches.length; i++) {
            var patch = patches[i];
            var node = nodeMap[patch.nodeId];
            if (!node) continue;
            applyPatch(node, patch);
        }
    }

    function applyPatch(node, patch) {
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
                applyPatches(patch.subPatches);
                break;
        }
    }

    // --- Redraw: replace entire node with new tree ---
    function execRedraw(node, patch) {
        var tree = JSON.parse(textDecoder.decode(patch.treeContent));
        var newNode = buildDOM(tree);
        if (newNode && node.parentNode) {
            // Clean old node from nodeMap
            cleanNodeMap(node);
            node.parentNode.replaceChild(newNode, node);
        }
    }

    // --- Text: update text content ---
    function execText(node, patch) {
        if (node.nodeType === 3) {
            node.nodeValue = patch.text;
        } else {
            node.textContent = patch.text;
        }
    }

    // --- Facts: apply property/attribute/style/event changes ---
    function execFacts(node, patch) {
        if (!patch.facts || !patch.facts.length) return;
        var diff = JSON.parse(textDecoder.decode(patch.facts));
        applyFactsDiff(node, diff, patch.nodeId);
    }

    // --- Append: add children from tree descriptions ---
    function execAppend(node, patch) {
        var trees = JSON.parse(textDecoder.decode(patch.treeContent));
        for (var i = 0; i < trees.length; i++) {
            var child = buildDOM(trees[i]);
            if (child) node.appendChild(child);
        }
    }

    // --- RemoveLast: remove N children from end ---
    function execRemoveLast(node, patch) {
        var count = patch.count;
        for (var i = 0; i < count; i++) {
            if (node.lastChild) {
                cleanNodeMap(node.lastChild);
                node.removeChild(node.lastChild);
            }
        }
    }

    // --- Reorder: keyed children insert/remove/move ---
    function execReorder(node, patch) {
        if (!patch.reorder || !patch.reorder.length) return;
        var data = JSON.parse(textDecoder.decode(patch.reorder));

        // Phase 1: Identify which removes are moves (have matching insert key without tree).
        var moveKeys = {};
        if (data.ins) {
            for (var m = 0; m < data.ins.length; m++) {
                if (!data.ins[m].tree) {
                    moveKeys[data.ins[m].k] = true;
                }
            }
        }

        // Phase 2: Detach removed nodes.
        var stashed = {};
        if (data.rem) {
            for (var i = 0; i < data.rem.length; i++) {
                var rem = data.rem[i];
                var child = node.childNodes[rem.i];
                if (child) {
                    if (moveKeys[rem.k]) {
                        stashed[rem.k] = child;
                        node.removeChild(child);
                    } else {
                        cleanNodeMap(child);
                        node.removeChild(child);
                    }
                }
            }
        }

        // Phase 3: Apply inserts.
        if (data.ins) {
            for (var j = 0; j < data.ins.length; j++) {
                var ins = data.ins[j];
                var newChild;
                if (ins.tree) {
                    // New node: build from tree description.
                    newChild = buildDOM(ins.tree);
                } else if (stashed[ins.k]) {
                    // Move: reuse stashed DOM node.
                    newChild = stashed[ins.k];
                    delete stashed[ins.k];
                }
                if (newChild) {
                    var ref = node.childNodes[ins.i] || null;
                    node.insertBefore(newChild, ref);
                }
            }
        }

        // Phase 4: Apply sub-patches for changed children.
        if (patch.subPatches) {
            applyPatches(patch.subPatches);
        }
    }

    // --- Plugin: forward data to JS plugin handler ---
    function execPlugin(node, patch) {
        if (!patch.pluginData || !patch.pluginData.length) return;
        var data = JSON.parse(textDecoder.decode(patch.pluginData));
        var nid = patch.nodeId;
        // Find plugin name — stored as data attribute during buildDOM
        var pluginName = node._godomPlugin;
        var handler = window.godom && window.godom._plugins && window.godom._plugins[pluginName];
        if (!handler) return;

        if (!pluginState[nid]) {
            handler.init(node, data);
            pluginState[nid] = true;
        } else {
            handler.update(node, data);
        }
    }

    // =========================================================================
    // 5. Facts application — properties, attributes, styles, events
    // =========================================================================

    function applyFactsDiff(el, diff, nodeId) {
        // Properties
        if (diff.p) {
            for (var key in diff.p) {
                var val = diff.p[key];
                if (val === null || val === undefined) {
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
                    delete eventMap[nodeId + ":" + key];
                } else {
                    // evtData has {on, key, msg, sp, pd}
                    registerNodeEvents(nodeId, el, [evtData]);
                }
            }
        }
    }

    // =========================================================================
    // 6. Event handling — wire DOM events to send messages back to Go
    // =========================================================================

    function sendEnvelope(msg, args, value) {
        var env = {msg: msg};
        if (args) env.args = args;
        if (value) env.value = value;
        ws.send(Proto.Envelope.encode(env).finish());
    }

    // Register event listeners on a DOM element using node ID for keying.
    function registerNodeEvents(nodeId, el, evts) {
        if (!evts) return;
        for (var i = 0; i < evts.length; i++) {
            var e = evts[i];
            var evtName = e.on;

            var key = nodeId + ":" + evtName;
            if (evtName === "keydown" && e.key) {
                key += ":" + e.key;
            }
            eventMap[key] = e;

            // Skip if listener already attached for this event type
            if (el._godomEvt && el._godomEvt[evtName]) continue;
            if (!el._godomEvt) el._godomEvt = {};
            el._godomEvt[evtName] = true;

            if (evtName === "input") {
                (function(k, elem) {
                    elem.addEventListener("input", function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        var valBytes = textEncoder.encode(JSON.stringify(elem.value));
                        sendEnvelope(ev.msg, null, valBytes);
                    });
                })(key, el);

            } else if (evtName === "keydown") {
                (function(nid, elem) {
                    elem.addEventListener("keydown", function(ke) {
                        var ev = eventMap[nid + ":keydown:" + ke.key]
                              || eventMap[nid + ":keydown"];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(nodeId, el);

            } else if (evtName === "mousedown" || evtName === "mouseup") {
                (function(k, elem, en) {
                    elem.addEventListener(en, function(me) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [me.offsetX, me.offsetY]);
                    });
                })(key, el, evtName);

            } else if (evtName === "mousemove") {
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

            } else if (evtName === "wheel") {
                (function(k, elem) {
                    elem.addEventListener("wheel", function(we) {
                        we.preventDefault();
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [we.deltaY]);
                    }, {passive: false});
                })(key, el);

            } else if (evtName === "drop") {
                setupDropHandler(key, el, e.key || "");

            } else {
                (function(k, elem, en) {
                    elem.addEventListener(en, function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(key, el, evtName);
            }
        }
    }

    // =========================================================================
    // 7. Drag & drop
    // =========================================================================

    function setupDraggable(el, group, value) {
        el.setAttribute("draggable", "true");
        el.dataset.gDrag = value || "";
        el.dataset.gDragGroup = group || "";
        if (!el._godomDragStart) {
            el._godomDragStart = true;
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

    // Remove a DOM node and clean up all nodeMap references.
    function cleanNodeMap(node) {
        if (!node) return;
        // Remove this node's ID from nodeMap
        for (var id in nodeMap) {
            if (nodeMap[id] === node) {
                delete nodeMap[id];
                delete pluginState[id];
                // Clean eventMap entries for this node ID
                for (var key in eventMap) {
                    if (key.indexOf(id + ":") === 0) {
                        delete eventMap[key];
                    }
                }
                break;
            }
        }
        // Recurse into children
        if (node.childNodes) {
            for (var i = 0; i < node.childNodes.length; i++) {
                cleanNodeMap(node.childNodes[i]);
            }
        }
    }

    // =========================================================================
    // 9. Plugin registration (global API)
    // =========================================================================

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
