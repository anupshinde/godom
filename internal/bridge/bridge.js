// bridge.js — godom VDOM bridge.
//
// Receives binary protobuf VDomMessage from Go over WebSocket.
// On "init": builds DOM from tree description, registers events.
// On "patch": applies minimal DOM mutations using nodeMap[id] lookups.
//
// Each render target is identified by name (from g-component attribute).
// The root component has name "document.body" and renders into document.body.
// A named component can have multiple DOM targets — each gets its own
// encapsulated context (own nodeMap, own pluginState).
//
// Structure:
//   1. State & globals
//   2. Connection — WebSocket with auto-reconnect and disconnect overlay
//   3. Target management — per-target encapsulated contexts
//   4. DOM construction — build DOM nodes from tree descriptions
//   5. Patch execution — dispatch by op type
//   6. Facts application — properties, attributes, styles, events
//   7. Event handling — drag/drop, input sync, method calls
//   8. Helpers
//   9. Plugin registration (global API)

(function() {

    // =========================================================================
    // 1. State & globals
    // =========================================================================

    var ws;
    var targets = {};   // targetName (string) → [target context, ...]

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
            var name = msg.targetName || "";

            if (msg.type === "init") {
                hideDisconnectOverlay();
                reconnectDelay = 1000;
                initTarget(name, msg);
            } else if (msg.type === "patch") {
                var ctxList = targets[name];
                if (!ctxList || ctxList.length === 0) {
                    console.warn("[godom patch] no target context for name=" + name);
                    return;
                }
                for (var i = 0; i < ctxList.length; i++) {
                    ctxList[i].applyPatches(msg.patches);
                }
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
    // 3. Target management — per-target encapsulated contexts
    // =========================================================================

    // initTarget creates encapsulated contexts for a named component and
    // builds the initial DOM tree inside each target element.
    function initTarget(name, msg) {
        if (name === "document.body") {
            // Root component: render into document.body.
            // Destroy all existing target contexts first.
            for (var n in targets) {
                if (targets.hasOwnProperty(n)) {
                    var list = targets[n];
                    for (var i = 0; i < list.length; i++) {
                        list[i].cleanup();
                    }
                }
            }
            targets = {};
            var ctx = createTargetContext(name, document.body);
            targets[name] = [ctx];
            ctx.init(msg);
        } else {
            // Named component: find all elements with g-component="name".
            var els = document.querySelectorAll('[g-component="' + name + '"]');
            if (els.length === 0) {
                console.warn('godom: no target found for component "' + name + '" — check that the parent is mounted first and the g-component attribute matches');
                return;
            }
            // Clean up existing contexts for this name.
            if (targets[name]) {
                for (var i = 0; i < targets[name].length; i++) {
                    targets[name][i].cleanup();
                }
            }
            var ctxList = [];
            for (var i = 0; i < els.length; i++) {
                var ctx = createTargetContext(name, els[i]);
                ctxList.push(ctx);
                ctx.init(msg);
            }
            targets[name] = ctxList;
        }
    }

    // createTargetContext builds an encapsulated closure for a render target.
    // All DOM state (nodeMap, pluginState) is private to this closure.
    function createTargetContext(name, targetEl) {
        var nodeMap = {};
        var pluginState = {};
        var pendingPluginInits = [];

        // --- DOM construction ---

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

            applyProps(el, tree.p);
            applyAttrs(el, tree.a);
            applyAttrsNS(el, tree.an);
            applyStyles(el, tree.s);

            if (tree.ev) {
                registerEvents(tree.id, el, tree.ev);
            }

            if (!tree.ns && tree.tag) {
                autoRegisterInputSync(tree.id, el, tree.tag);
            }

            autoRegisterDraggable(el);

            if (tree.c) {
                for (var i = 0; i < tree.c.length; i++) {
                    var child = buildDOM(tree.c[i]);
                    if (child) el.appendChild(child);
                }
            }

            if (tree.tag === "select" && tree.p && tree.p.value !== undefined) {
                deferSelectValue(el, tree.p.value);
            }

            if (tree.plug) {
                el._godomPlugin = tree.plug;
                var handler = window.godom && window.godom._plugins && window.godom._plugins[tree.plug];
                if (handler && tree.pd !== undefined) {
                    pendingPluginInits.push({el: el, id: tree.id, handler: handler, data: tree.pd});
                }
            }

            return el;
        }

        // --- Patch execution ---

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
                if (!node) {
                    console.warn("[godom patch] skip: nodeMap[" + patch.nodeId + "] not found for op=" + patch.op + " in target " + name);
                    continue;
                }
                applyPatch(node, patch);
            }

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

        function execRedraw(node, patch) {
            var tree = JSON.parse(textDecoder.decode(patch.treeContent));
            var newNode = buildDOM(tree);
            cleanNodeMap(node);
            if (newNode && node.parentNode) {
                node.parentNode.replaceChild(newNode, node);
            }
        }

        function execText(node, patch) {
            if (node.nodeType === 3) {
                node.nodeValue = patch.text;
            } else {
                node.textContent = patch.text;
            }
        }

        function execFacts(node, patch) {
            if (!patch.facts || !patch.facts.length) return;
            var diff = JSON.parse(textDecoder.decode(patch.facts));
            applyFactsDiff(node, diff, patch.nodeId);
        }

        function execAppend(node, patch) {
            var trees = JSON.parse(textDecoder.decode(patch.treeContent));
            for (var i = 0; i < trees.length; i++) {
                var child = buildDOM(trees[i]);
                if (child) node.appendChild(child);
            }
        }

        function execRemoveLast(node, patch) {
            var count = patch.count;
            for (var i = 0; i < count; i++) {
                if (node.lastChild) {
                    var victim = node.lastChild;
                    cleanNodeMap(victim);
                    node.removeChild(victim);
                }
            }
        }

        function execReorder(node, patch) {
            if (!patch.reorder || !patch.reorder.length) return;
            var data = JSON.parse(textDecoder.decode(patch.reorder));

            var moveKeys = {};
            if (data.ins) {
                for (var m = 0; m < data.ins.length; m++) {
                    if (!data.ins[m].tree) {
                        moveKeys[data.ins[m].k] = true;
                    }
                }
            }

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

            if (data.ins) {
                for (var j = 0; j < data.ins.length; j++) {
                    var ins = data.ins[j];
                    var newChild;
                    if (ins.tree) {
                        newChild = buildDOM(ins.tree);
                    } else if (stashed[ins.k]) {
                        newChild = stashed[ins.k];
                        delete stashed[ins.k];
                    }
                    if (newChild) {
                        var ref = node.childNodes[ins.i] || null;
                        node.insertBefore(newChild, ref);
                    }
                }
            }

            if (patch.subPatches) {
                applyPatches(patch.subPatches);
            }
        }

        function execPlugin(node, patch) {
            if (!patch.pluginData || !patch.pluginData.length) return;
            var data = JSON.parse(textDecoder.decode(patch.pluginData));
            var nid = patch.nodeId;
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

        // --- Facts application ---

        function applyFactsDiff(el, diff, nodeId) {
            if (diff.p) {
                for (var key in diff.p) {
                    var val = diff.p[key];
                    if (val === null || val === undefined) {
                        el[key] = "";
                    } else if (key === "value" && el.tagName === "SELECT") {
                        deferSelectValue(el, val);
                    } else if (key === "_scrollratio") {
                        (function(target, r) {
                            requestAnimationFrame(function() {
                                target.scrollTop = r * (target.scrollHeight - target.clientHeight);
                            });
                        })(el, val);
                    } else {
                        el[key] = val;
                    }
                }
            }

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

            if (diff.e) {
                for (var key in diff.e) {
                    var ev = diff.e[key];
                    if (ev) {
                        registerSingleEvent(nodeId, el, ev);
                    }
                }
            }

            if (el.tagName) {
                var tag = el.tagName.toLowerCase();
                autoRegisterInputSync(nodeId, el, tag);
            }

            autoRegisterDraggable(el);
        }

        // --- Event handling ---

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

            if (tag === "input" && el.type === "checkbox") {
                el.addEventListener("change", function() {
                    sendNodeEvent(nodeId, el.checked ? "true" : "false");
                });
            } else if (tag === "input" || tag === "textarea") {
                el.addEventListener("input", function() {
                    sendNodeEvent(nodeId, el.value);
                });
            } else if (tag === "select") {
                el.addEventListener("change", function() {
                    sendNodeEvent(nodeId, el.value);
                });
            }
        }

        var _currentDragGroup = "";
        function autoRegisterDraggable(el) {
            if (el._godomDrag) return;
            if (!el.draggable) return;
            el._godomDrag = true;

            el.addEventListener("dragstart", function(domEvent) {
                var value = el.getAttribute("data-drag-value") || "";
                _currentDragGroup = el.getAttribute("data-drag-group") || "";
                domEvent.dataTransfer.setData("text/plain", value);
                domEvent.dataTransfer.effectAllowed = "move";
                el.classList.add("g-dragging");
            });
            el.addEventListener("dragend", function() {
                el.classList.remove("g-dragging");
                _currentDragGroup = "";
            });
        }

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

            if (eventType === "drop") {
                var dropGroup = el.getAttribute("data-drop-group") || "";
                el.addEventListener("dragover", function(domEvent) {
                    if (dropGroup && dropGroup !== _currentDragGroup) return;
                    domEvent.preventDefault();
                    el.classList.add("g-drag-over");
                });
                el.addEventListener("dragleave", function() {
                    el.classList.remove("g-drag-over");
                });
                el.addEventListener("drop", function(domEvent) {
                    domEvent.preventDefault();
                    el.classList.remove("g-drag-over");
                    if (dropGroup && dropGroup !== _currentDragGroup) return;
                    domEvent.stopPropagation();
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

            var isThrottled = (eventType === "mousemove" || eventType === "scroll");
            var pendingFrame = 0;
            var latestDomEvent = null;

            el.addEventListener(eventType, function(domEvent) {
                if (ev.key && domEvent.key !== ev.key && domEvent.code !== ev.key) return;
                if (isThrottled) {
                    latestDomEvent = domEvent;
                    if (pendingFrame) return;
                    pendingFrame = requestAnimationFrame(function() {
                        pendingFrame = 0;
                        var de = latestDomEvent;
                        if (ev.sp) de.stopPropagation();
                        if (ev.pd) de.preventDefault();
                        var allArgs = ev.args ? ev.args.slice() : [];
                        if (eventType === "mousemove") {
                            allArgs.unshift(
                                textEncoder.encode(String(de.clientX)),
                                textEncoder.encode(String(de.clientY))
                            );
                        } else if (eventType === "scroll") {
                            allArgs.unshift(
                                textEncoder.encode(String(Math.round(el.scrollTop))),
                                textEncoder.encode(String(Math.round(el.scrollHeight))),
                                textEncoder.encode(String(Math.round(el.clientHeight)))
                            );
                        }
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
                if (eventType === "wheel") {
                    allArgs.unshift(
                        textEncoder.encode(String(domEvent.deltaY))
                    );
                }
                sendMethodCall(nodeId, ev.method, allArgs);
            });
        }

        // --- Helpers ---

        function cleanNodeMap(node) {
            if (!node) return;
            var id = node._godomId;
            if (id !== undefined) {
                delete nodeMap[id];
                delete pluginState[id];
            }
            if (node.childNodes) {
                for (var i = 0; i < node.childNodes.length; i++) {
                    cleanNodeMap(node.childNodes[i]);
                }
            }
        }

        // --- Public interface for this target context ---

        return {
            init: function(msg) {
                targetEl.innerHTML = "";

                var tree = JSON.parse(textDecoder.decode(msg.tree));
                if (tree) {
                    var domNode = buildDOM(tree);
                    if (domNode) {
                        if (tree.tag === "body") {
                            while (domNode.firstChild) {
                                targetEl.appendChild(domNode.firstChild);
                            }
                            nodeMap[tree.id] = targetEl;
                        } else {
                            targetEl.appendChild(domNode);
                        }
                    }
                }

                for (var pi = 0; pi < pendingPluginInits.length; pi++) {
                    var p = pendingPluginInits[pi];
                    p.handler.init(p.el, p.data);
                    pluginState[p.id] = true;
                }
                pendingPluginInits = [];
            },

            applyPatches: applyPatches,

            cleanup: function() {
                nodeMap = {};
                pluginState = {};
                pendingPluginInits = [];
            }
        };
    }

    // =========================================================================
    // Shared helpers
    // =========================================================================

    function applyProps(el, props) {
        if (!props) return;
        for (var key in props) {
            if (key === "_scrollratio") {
                var ratio = props[key];
                (function(target, r) {
                    requestAnimationFrame(function() {
                        target.scrollTop = r * (target.scrollHeight - target.clientHeight);
                    });
                })(el, ratio);
            } else {
                el[key] = props[key];
            }
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

    function deferSelectValue(el, val) {
        requestAnimationFrame(function() { el.value = val; });
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
