// bridge.js — godom VDOM bridge.
//
// Receives binary protobuf VDomMessage from Go over WebSocket.
// On "init": builds DOM from tree description, registers events.
// On "patch": applies minimal DOM mutations using nodeMap[id] lookups.
//
// Structure:
//   1. State & globals
//   2. Connection — WebSocket with auto-reconnect and disconnect overlay
//   3. DOM construction — build DOM nodes from tree descriptions
//   4. Patch execution — dispatch by op type
//   5. Facts application — properties, attributes, styles, events
//   6. Event handling — drag/drop, input sync, method calls
//   7. Helpers
//   8. Plugin registration (global API)

(function() {

    // =========================================================================
    // 1. State & globals
    // =========================================================================

    var ws;
    var nodeMap = {};       // node ID (int) → DOM node
    var pluginState = {};   // node ID → true if plugin init called
    var rootNode;           // the root DOM node (document.body)

    var Proto = godomProto;
    var textDecoder = new TextDecoder();
    var textEncoder = new TextEncoder();

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
            if (tree.id) {
                nodeMap[tree.id] = textNode;
                textNode._godomId = tree.id;
            }
            return textNode;
        }

        // Element node (including keyed)
        var el;
        if (tree.ns) {
            el = document.createElementNS(tree.ns, tree.tag);
        } else {
            el = document.createElement(tree.tag);
        }

        if (tree.id) {
            nodeMap[tree.id] = el;
            el._godomId = tree.id;
        }

        // Apply facts: props, attrs, styles, namespaced attrs
        applyProps(el, tree.p);
        applyAttrs(el, tree.a);
        applyAttrsNS(el, tree.an);
        applyStyles(el, tree.s);

        // Layer 2: register event listeners from tree
        if (tree.ev) {
            registerEvents(tree.id, el, tree.ev);
        }

        // Layer 1: auto-sync input values back to Go DOM view (Tree)
        if (!tree.ns && tree.tag) {
            autoRegisterInputSync(tree.id, el, tree.tag);
        }

        // Wire up HTML5 drag if element has draggable prop
        autoRegisterDraggable(el);

        // Build children
        if (tree.c) {
            for (var i = 0; i < tree.c.length; i++) {
                var child = buildDOM(tree.c[i]);
                if (child) el.appendChild(child);
            }
        }

        // Plugin init
        if (tree.plug) {
            el._godomPlugin = tree.plug;
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

        var focusedEl = document.activeElement;
        var selStart = null, selEnd = null;
        if (focusedEl && focusedEl.setSelectionRange) {
            try { selStart = focusedEl.selectionStart; selEnd = focusedEl.selectionEnd; } catch(e) {}
        }

        for (var i = 0; i < patches.length; i++) {
            var patch = patches[i];
            var node = nodeMap[patch.nodeId];
            if (!node) continue;
            applyPatch(node, patch);
        }

        // Restore focus only if it was lost during patching
        if (focusedEl && focusedEl !== document.activeElement) {
            focusedEl.focus();
            if (selStart !== null && focusedEl.setSelectionRange) {
                try { focusedEl.setSelectionRange(selStart, selEnd); } catch(e) {}
            }
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

        // Layer 2: register new events from facts diff
        if (diff.e) {
            for (var key in diff.e) {
                var ev = diff.e[key];
                if (ev) {
                    registerSingleEvent(nodeId, el, ev);
                }
            }
        }

        // Layer 1: re-register input sync if this element was redrawn
        if (el.tagName) {
            var tag = el.tagName.toLowerCase();
            autoRegisterInputSync(nodeId, el, tag);
        }

        // Re-register draggable if props changed
        autoRegisterDraggable(el);
    }

    // =========================================================================
    // 6. Event handling
    // =========================================================================

    // --- Layer 1: auto-sync input values via NodeEvent (tag byte 0x01) ---

    function sendNodeEvent(nodeId, value) {
        var msg = Proto.NodeEvent.encode({nodeId: nodeId, value: value}).finish();
        var tagged = new Uint8Array(1 + msg.length);
        tagged[0] = 1;
        tagged.set(msg, 1);
        ws.send(tagged);
    }

    function autoRegisterInputSync(nodeId, el, tag) {
        if (el._godomSync) return;
        el._godomSync = true;

        if (tag === "input" || tag === "textarea") {
            el.addEventListener("input", function() {
                sendNodeEvent(nodeId, el.value);
            });
        } else if (tag === "select") {
            el.addEventListener("change", function() {
                sendNodeEvent(nodeId, el.value);
            });
        }
    }

    // Any element with draggable="true" gets dataTransfer wiring.
    // Reads data-drag-value attr and puts it into dataTransfer on dragstart.
    function autoRegisterDraggable(el) {
        if (el._godomDrag) return;
        if (!el.draggable) return;
        el._godomDrag = true;

        el.addEventListener("dragstart", function(domEvent) {
            var value = el.getAttribute("data-drag-value") || "";
            domEvent.dataTransfer.setData("text/plain", value);
            domEvent.dataTransfer.effectAllowed = "move";
            el.classList.add("g-dragging");
        });
        el.addEventListener("dragend", function() {
            el.classList.remove("g-dragging");
        });
    }

    // --- Layer 2: method dispatch via MethodCall (tag byte 0x02) ---

    function sendMethodCall(nodeId, method, args) {
        var msg = Proto.MethodCall.encode({
            nodeId: nodeId,
            method: method,
            args: args
        }).finish();
        var tagged = new Uint8Array(1 + msg.length);
        tagged[0] = 2;
        tagged.set(msg, 1);
        ws.send(tagged);
    }

    function registerEvents(nodeId, el, events) {
        for (var i = 0; i < events.length; i++) {
            registerSingleEvent(nodeId, el, events[i]);
        }
    }

    function registerSingleEvent(nodeId, el, ev) {
        var eventType = ev.on;
        var listenerKey = "_godom_ev_" + eventType + (ev.key ? "_" + ev.key : "");

        if (el[listenerKey]) return;
        el[listenerKey] = true;

        // drop: read drag source value from dataTransfer, prepend to method args
        if (eventType === "drop") {
            el.addEventListener("dragover", function(domEvent) {
                domEvent.preventDefault();
                el.classList.add("g-drag-over");
            });
            el.addEventListener("dragleave", function() {
                el.classList.remove("g-drag-over");
            });
            el.addEventListener("drop", function(domEvent) {
                domEvent.preventDefault();
                el.classList.remove("g-drag-over");
                var sourceValue = domEvent.dataTransfer.getData("text/plain") || "null";
                var targetValue = el.getAttribute("data-drag-value") || "null";
                var args = [
                    textEncoder.encode(sourceValue),
                    textEncoder.encode(targetValue)
                ];
                for (var a = 0; a < (ev.args || []).length; a++) {
                    args.push(ev.args[a]);
                }
                sendMethodCall(nodeId, ev.method, args);
            });
            return;
        }

        // Throttle mousemove: dispatch at most once per animation frame.
        var isMouseMove = (eventType === "mousemove");
        var pendingFrame = 0;
        var latestDomEvent = null;

        el.addEventListener(eventType, function(domEvent) {
            if (ev.key && domEvent.key !== ev.key) return;
            if (isMouseMove) {
                latestDomEvent = domEvent;
                if (pendingFrame) return;
                pendingFrame = requestAnimationFrame(function() {
                    pendingFrame = 0;
                    var de = latestDomEvent;
                    if (ev.sp) de.stopPropagation();
                    if (ev.pd) de.preventDefault();
                    var allArgs = ev.args ? ev.args.slice() : [];
                    allArgs.unshift(
                        textEncoder.encode(String(de.clientX)),
                        textEncoder.encode(String(de.clientY))
                    );
                    sendMethodCall(nodeId, ev.method, allArgs);
                });
                return;
            }
            if (ev.sp) domEvent.stopPropagation();
            if (ev.pd) domEvent.preventDefault();
            var allArgs = ev.args ? ev.args.slice() : [];
            if (eventType === "mousedown" || eventType === "mouseup") {
                allArgs.unshift(
                    textEncoder.encode(String(domEvent.clientX)),
                    textEncoder.encode(String(domEvent.clientY))
                );
            }
            sendMethodCall(nodeId, ev.method, allArgs);
        });
    }

    // =========================================================================
    // 7. Helpers
    // =========================================================================

    // Remove a DOM node and clean up all nodeMap references.
    function cleanNodeMap(node) {
        if (!node) return;
        // Remove this node's ID from nodeMap via stored _godomId
        var id = node._godomId;
        if (id !== undefined) {
            delete nodeMap[id];
            delete pluginState[id];
        }
        // Recurse into children
        if (node.childNodes) {
            for (var i = 0; i < node.childNodes.length; i++) {
                cleanNodeMap(node.childNodes[i]);
            }
        }
    }

    // =========================================================================
    // 8. Plugin registration (global API)
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
