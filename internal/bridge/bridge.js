// bridge.js — godom bridge.
//
// Receives binary protobuf ServerMessage from Go over WebSocket.
// On init: builds DOM from tree description, registers events.
// On patch: applies minimal DOM mutations using nodeMap[id] lookups.
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

    const nsName = window.GODOM_NS || "godom";
    const ns = window[nsName] = window[nsName] || {};
    if (!ns._plugins) ns._plugins = {};

    let ws;
    let targets = {};      // targetName (string) → [target context, ...]
    const readyEls = new WeakSet(); // DOM elements that have been initialized

    const Proto = godomProto;
    const SK = Proto.ServerKind;   // cached enum constants for fast dispatch
    const BK = Proto.BrowserKind;
    const textDecoder = new TextDecoder();
    const textEncoder = new TextEncoder();

    // =========================================================================
    // 2. Connection — WebSocket with auto-reconnect and disconnect overlay
    // =========================================================================

    let overlay = null;
    let hasRoot = false; // true when document.body is a godom root

    function showDisconnectOverlay(errorMsg) {
        if (hasRoot) {
            // godom owns the page — full-page overlay.
            if (overlay) return;
            overlay = document.createElement("div");
            overlay.innerHTML = window.GODOM_DISCONNECT_HTML || "";
            if (errorMsg) {
                const errorEl = overlay.querySelector(".godom-disconnect-error");
                if (errorEl) {
                    errorEl.style.display = "";
                    const pre = errorEl.querySelector("pre");
                    if (pre) pre.textContent = errorMsg;
                }
            }
            document.body.appendChild(overlay);
        } else {
            // Embedded — dim each component target and show a small badge.
            for (const n in targets) {
                if (!targets.hasOwnProperty(n)) continue;
                const list = targets[n];
                for (let i = 0; i < list.length; i++) {
                    const el = list[i].targetEl;
                    if (!el || el._godomDisconnected) continue;
                    el._godomDisconnected = true;
                    el.style.opacity = "0.4";
                    el.style.pointerEvents = "none";
                    // Badge as sibling so parent opacity doesn't affect it.
                    const wrapper = document.createElement("div");
                    wrapper.className = "godom-disconnect-badge";
                    wrapper.innerHTML = window.GODOM_DISCONNECT_BADGE || "";
                    if (el.parentNode) el.parentNode.insertBefore(wrapper, el.nextSibling);
                }
            }
        }
    }

    function hideDisconnectOverlay() {
        if (overlay) {
            overlay.remove();
            overlay = null;
        }
        // Clean up per-target disconnect badges and restore target styles.
        const badges = document.querySelectorAll(".godom-disconnect-badge");
        for (let i = 0; i < badges.length; i++) {
            badges[i].remove();
        }
        for (const n in targets) {
            if (!targets.hasOwnProperty(n)) continue;
            const list = targets[n];
            for (let i = 0; i < list.length; i++) {
                const el = list[i].targetEl;
                if (!el) continue;
                el.style.opacity = "";
                el.style.pointerEvents = "";
                delete el._godomDisconnected;
            }
        }
    }

    let reconnectDelay = 1000;

    function connect() {
        const wsUrl = window.GODOM_WS_URL || (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "__GODOM_WS_PATH__";
        let firedOnConnect = false;
        ws = new WebSocket(wsUrl);
        ws.binaryType = "arraybuffer";

        ws.onopen = () => {
            hideDisconnectOverlay();
            reconnectDelay = 1000;
            // Clean up all existing target contexts so scanAndRequestComponents
            // treats every [g-component] as fresh. This handles reconnect after
            // server restart (state lost) or long disconnect (state diverged).
            cleanupAllTargets();
            if (!firedOnConnect && ns.onconnect) {
                firedOnConnect = true;
                ns.onconnect();
            }
            // In embedded mode, scan for g-component targets immediately —
            // no SERVER_INIT will arrive to trigger it. In root mode, the
            // static HTML may contain unresolved template expressions
            // (e.g. {{slot.Name}}) so we wait for SERVER_INIT to render
            // the real DOM first.
            if (!window.GODOM_ROOT) {
                scanAndRequestComponents();
            }
        };

        ws.onmessage = (evt) => {
            const msg = Proto.ServerMessage.decode(new Uint8Array(evt.data));

            switch (msg.kind) {
            case SK.SERVER_INIT:
                initTarget(msg.target || "", msg);
                scanAndRequestComponents();
                break;
            case SK.SERVER_PATCH: {
                const name = msg.target || "";
                const ctxList = targets[name];
                if (!ctxList || ctxList.length === 0) {
                    console.warn(`[godom patch] no target context for name=${name}`);
                    return;
                }
                for (let i = 0; i < ctxList.length; i++) {
                    ctxList[i].applyPatches(msg.patches);
                }
                break;
            }
            case SK.SERVER_JSCALL:
                handleJSCall(msg);
                break;
            }
        };

        ws.onclose = (evt) => {
            firedOnConnect = false;
            const errorMsg = evt.reason || null;
            if (errorMsg && window.GODOM_DEBUG) console.error("[godom]", errorMsg);
            showDisconnectOverlay(errorMsg);
            if (ns.ondisconnect) ns.ondisconnect(errorMsg);
            if (!errorMsg) {
                setTimeout(connect, reconnectDelay);
                reconnectDelay = Math.min(reconnectDelay * 2, 30000);
            }
        };

        ws.onerror = (evt) => {
            if (ns.onerror) ns.onerror(evt);
            ws.close();
        };
    }

    // =========================================================================
    // 3. Target management — per-target encapsulated contexts
    // =========================================================================

    // sendInitRequest sends a BROWSER_INIT_REQUEST for the given component name.
    function sendInitRequest(name) {
        const msg = Proto.BrowserMessage.encode({
            kind: BK.BROWSER_INIT_REQUEST,
            component: name
        }).finish();
        ws.send(msg);
    }

    // scanAndRequestComponents finds all [g-component] elements not yet
    // initialized and sends init requests to the server for each unique
    // component name.
    function scanAndRequestComponents() {
        const els = document.querySelectorAll("[g-component]");
        const requested = {};
        for (let i = 0; i < els.length; i++) {
            if (readyEls.has(els[i])) continue;
            const name = els[i].getAttribute("g-component");
            if (name && !requested[name]) {
                requested[name] = true;
                sendInitRequest(name);
            }
        }
    }

    // cleanupAllTargets destroys all existing target contexts, clearing
    // readyEls and removing .g-ready so components can be re-initialized.
    function cleanupAllTargets() {
        for (const n in targets) {
            if (targets.hasOwnProperty(n)) {
                const list = targets[n];
                for (let i = 0; i < list.length; i++) {
                    list[i].cleanup();
                }
            }
        }
        targets = {};
    }

    // initTarget creates encapsulated contexts for a named component and
    // builds the initial DOM tree inside each target element.
    function initTarget(name, msg) {
        if (name === "document.body") {
            // Root component: render into document.body.
            hasRoot = true;
            cleanupAllTargets();
            const ctx = createTargetContext(name, document.body);
            targets[name] = [ctx];
            ctx.init(msg);
            document.body.classList.add("g-ready");
        } else {
            // Named component: find all elements with g-component="name".
            const els = document.querySelectorAll(`[g-component="${name}"]`);
            if (els.length === 0) {
                if (window.GODOM_DEBUG) console.warn(`godom: no target found for component "${name}" — check that the parent is mounted first and the g-component attribute matches`);
                return;
            }
            // Clean up existing contexts for this name.
            if (targets[name]) {
                for (let i = 0; i < targets[name].length; i++) {
                    targets[name][i].cleanup();
                }
            }
            const ctxList = [];
            for (let i = 0; i < els.length; i++) {
                const ctx = createTargetContext(name, els[i]);
                ctxList.push(ctx);
                ctx.init(msg);
                els[i].classList.add("g-ready");
                readyEls.add(els[i]);
            }
            targets[name] = ctxList;
        }
    }

    // createTargetContext builds an encapsulated closure for a render target.
    // All DOM state (nodeMap, pluginState) is private to this closure.
    function createTargetContext(name, targetEl) {
        let nodeMap = {};
        let pluginState = {};
        let pendingPluginInits = [];
        let hasNewComponents = false;
        const useShadow = targetEl !== document.body && targetEl.hasAttribute("g-shadow");
        const renderRoot = useShadow ? targetEl.attachShadow({mode: "open"}) : targetEl;

        // --- DOM construction ---

        function buildDOM(tree) {
            if (!tree) return null;

            if (tree.t === "text") {
                const textNode = document.createTextNode(tree.x || "");
                if (tree.id) {
                    nodeMap[tree.id] = textNode;
                    textNode._godomId = tree.id;
                }
                return textNode;
            }

            let el;
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
            if (tree.a && tree.a["g-component"]) {
                hasNewComponents = true;
            }
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
                for (let i = 0; i < tree.c.length; i++) {
                    const child = buildDOM(tree.c[i]);
                    if (child) el.appendChild(child);
                }
            }

            if (tree.tag === "select" && tree.p && tree.p.value !== undefined) {
                deferSelectValue(el, tree.p.value);
            }

            if (tree.plug) {
                el._godomPlugin = tree.plug;
                const handler = ns._plugins[tree.plug];
                if (handler && tree.pd !== undefined) {
                    pendingPluginInits.push({el: el, id: tree.id, handler: handler, data: tree.pd});
                }
            }

            return el;
        }

        // --- Patch execution ---

        function applyPatches(patches) {
            if (!patches) return;

            const focusedEl = document.activeElement;
            let selStart = null, selEnd = null;
            if (focusedEl && focusedEl.setSelectionRange) {
                try { selStart = focusedEl.selectionStart; selEnd = focusedEl.selectionEnd; } catch(e) {}
            }

            for (let i = 0; i < patches.length; i++) {
                const patch = patches[i];
                const node = nodeMap[patch.nodeId];
                if (!node) {
                    console.warn(`[godom patch] skip: nodeMap[${patch.nodeId}] not found for op=${patch.op} in target ${name}`);
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
            const tree = JSON.parse(textDecoder.decode(patch.treeContent));
            const newNode = buildDOM(tree);
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
            const diff = JSON.parse(textDecoder.decode(patch.facts));
            applyFactsDiff(node, diff, patch.nodeId);
        }

        function execAppend(node, patch) {
            const trees = JSON.parse(textDecoder.decode(patch.treeContent));
            for (let i = 0; i < trees.length; i++) {
                const child = buildDOM(trees[i]);
                if (child) node.appendChild(child);
            }
        }

        function execRemoveLast(node, patch) {
            const count = patch.count;
            for (let i = 0; i < count; i++) {
                if (node.lastChild) {
                    const victim = node.lastChild;
                    cleanNodeMap(victim);
                    node.removeChild(victim);
                }
            }
        }

        function execReorder(node, patch) {
            if (!patch.reorder || !patch.reorder.length) return;
            const data = JSON.parse(textDecoder.decode(patch.reorder));

            const moveKeys = {};
            if (data.ins) {
                for (let m = 0; m < data.ins.length; m++) {
                    if (!data.ins[m].tree) {
                        moveKeys[data.ins[m].k] = true;
                    }
                }
            }

            const stashed = {};
            if (data.rem) {
                for (let i = 0; i < data.rem.length; i++) {
                    const rem = data.rem[i];
                    const child = node.childNodes[rem.i];
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
                for (let j = 0; j < data.ins.length; j++) {
                    const ins = data.ins[j];
                    let newChild;
                    if (ins.tree) {
                        newChild = buildDOM(ins.tree);
                    } else if (stashed[ins.k]) {
                        newChild = stashed[ins.k];
                        delete stashed[ins.k];
                    }
                    if (newChild) {
                        const ref = node.childNodes[ins.i] || null;
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
            const data = JSON.parse(textDecoder.decode(patch.pluginData));
            const nid = patch.nodeId;
            const pluginName = node._godomPlugin;
            const handler = ns._plugins[pluginName];
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
                for (const key in diff.p) {
                    const val = diff.p[key];
                    if (val === null || val === undefined) {
                        el[key] = "";
                    } else if (key === "value" && el.tagName === "SELECT") {
                        deferSelectValue(el, val);
                    } else if (key === "_scrollratio") {
                        ((target, r) => {
                            requestAnimationFrame(() => {
                                target.scrollTop = r * (target.scrollHeight - target.clientHeight);
                            });
                        })(el, val);
                    } else {
                        el[key] = val;
                    }
                }
            }

            if (diff.a) {
                for (const key in diff.a) {
                    const val = diff.a[key];
                    if (val === "") {
                        el.removeAttribute(key);
                    } else {
                        el.setAttribute(key, val);
                    }
                }
            }

            if (diff.an) {
                for (const key in diff.an) {
                    const nsAttr = diff.an[key];
                    if (!nsAttr || (!nsAttr.ns && !nsAttr.v)) {
                        el.removeAttributeNS(null, key);
                    } else {
                        el.setAttributeNS(nsAttr.ns, key, nsAttr.v);
                    }
                }
            }

            if (diff.s) {
                for (const key in diff.s) {
                    const val = diff.s[key];
                    if (val === "") {
                        el.style.removeProperty(key);
                    } else {
                        el.style.setProperty(key, val);
                    }
                }
            }

            if (diff.e) {
                for (const key in diff.e) {
                    const ev = diff.e[key];
                    if (ev) {
                        registerSingleEvent(nodeId, el, ev);
                    }
                }
            }

            if (el.tagName) {
                const tag = el.tagName.toLowerCase();
                autoRegisterInputSync(nodeId, el, tag);
            }

            autoRegisterDraggable(el);
        }

        // --- Event handling ---

        function sendNodeEvent(nodeId, value) {
            const msg = Proto.BrowserMessage.encode({
                kind: BK.BROWSER_INPUT,
                nodeId: nodeId,
                value: value
            }).finish();
            ws.send(msg);
        }

        function autoRegisterInputSync(nodeId, el, tag) {
            if (el._godomSync) return;
            el._godomSync = true;

            if (tag === "input" && el.type === "checkbox") {
                el.addEventListener("change", () => {
                    sendNodeEvent(nodeId, el.checked ? "true" : "false");
                });
            } else if (tag === "input" || tag === "textarea") {
                el.addEventListener("input", () => {
                    sendNodeEvent(nodeId, el.value);
                });
            } else if (tag === "select") {
                el.addEventListener("change", () => {
                    sendNodeEvent(nodeId, el.value);
                });
            }
        }

        let _currentDragGroup = "";
        function autoRegisterDraggable(el) {
            if (el._godomDrag) return;
            if (!el.draggable) return;
            el._godomDrag = true;

            el.addEventListener("dragstart", (domEvent) => {
                const value = el.getAttribute("data-drag-value") || "";
                _currentDragGroup = el.getAttribute("data-drag-group") || "";
                domEvent.dataTransfer.setData("text/plain", value);
                domEvent.dataTransfer.effectAllowed = "move";
                el.classList.add("g-dragging");
            });
            el.addEventListener("dragend", () => {
                el.classList.remove("g-dragging");
                _currentDragGroup = "";
            });
        }

        function sendMethodCall(nodeId, method, args) {
            const msg = Proto.BrowserMessage.encode({
                kind: BK.BROWSER_METHOD,
                nodeId: nodeId,
                method: method,
                args: args
            }).finish();
            ws.send(msg);
        }

        function registerEvents(nodeId, el, events) {
            for (let i = 0; i < events.length; i++) {
                registerSingleEvent(nodeId, el, events[i]);
            }
        }

        function registerSingleEvent(nodeId, el, ev) {
            const eventType = ev.on;
            const listenerKey = `_godom_ev_${eventType}${ev.key ? `_${ev.key}` : ""}`;

            if (el[listenerKey]) return;
            el[listenerKey] = true;

            if (eventType === "drop") {
                const dropGroup = el.getAttribute("data-drop-group") || "";
                el.addEventListener("dragover", (domEvent) => {
                    if (dropGroup && dropGroup !== _currentDragGroup) return;
                    domEvent.preventDefault();
                    el.classList.add("g-drag-over");
                });
                el.addEventListener("dragleave", () => {
                    el.classList.remove("g-drag-over");
                });
                el.addEventListener("drop", (domEvent) => {
                    domEvent.preventDefault();
                    el.classList.remove("g-drag-over");
                    if (dropGroup && dropGroup !== _currentDragGroup) return;
                    domEvent.stopPropagation();
                    const sourceValue = domEvent.dataTransfer.getData("text/plain") || "null";
                    const targetValue = el.getAttribute("data-drag-value") || "null";
                    const args = [
                        textEncoder.encode(sourceValue),
                        textEncoder.encode(targetValue)
                    ];
                    for (let a = 0; a < (ev.args || []).length; a++) {
                        args.push(ev.args[a]);
                    }
                    sendMethodCall(nodeId, ev.method, args);
                });
                return;
            }

            const isThrottled = (eventType === "mousemove" || eventType === "scroll");
            let pendingFrame = 0;
            let latestDomEvent = null;

            el.addEventListener(eventType, (domEvent) => {
                if (ev.key && domEvent.key !== ev.key && domEvent.code !== ev.key) return;
                if (isThrottled) {
                    latestDomEvent = domEvent;
                    if (pendingFrame) return;
                    pendingFrame = requestAnimationFrame(() => {
                        pendingFrame = 0;
                        const de = latestDomEvent;
                        if (ev.sp) de.stopPropagation();
                        if (ev.pd) de.preventDefault();
                        const allArgs = ev.args ? ev.args.slice() : [];
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
                const allArgs = ev.args ? ev.args.slice() : [];
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
            const id = node._godomId;
            if (id !== undefined) {
                delete nodeMap[id];
                delete pluginState[id];
            }
            if (node.childNodes) {
                for (let i = 0; i < node.childNodes.length; i++) {
                    cleanNodeMap(node.childNodes[i]);
                }
            }
        }

        // --- Public interface for this target context ---

        return {
            init: function(msg) {
                renderRoot.innerHTML = "";

                const tree = JSON.parse(textDecoder.decode(msg.tree));
                if (tree) {
                    const domNode = buildDOM(tree);
                    if (domNode) {
                        if (tree.tag === "body") {
                            while (domNode.firstChild) {
                                renderRoot.appendChild(domNode.firstChild);
                            }
                            nodeMap[tree.id] = renderRoot;
                        } else {
                            renderRoot.appendChild(domNode);
                        }
                    }
                }

                for (let pi = 0; pi < pendingPluginInits.length; pi++) {
                    const p = pendingPluginInits[pi];
                    p.handler.init(p.el, p.data);
                    pluginState[p.id] = true;
                }
                pendingPluginInits = [];
            },

            applyPatches: function(patches) {
                hasNewComponents = false;
                applyPatches(patches);
                if (hasNewComponents) {
                    scanAndRequestComponents();
                }
            },

            targetEl: targetEl,

            cleanup: function() {
                readyEls.delete(targetEl);
                targetEl.classList.remove("g-ready");
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
        for (const key in props) {
            if (key === "_scrollratio") {
                const ratio = props[key];
                ((target, r) => {
                    requestAnimationFrame(() => {
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
        for (const key in attrs) {
            el.setAttribute(key, attrs[key]);
        }
    }

    function applyAttrsNS(el, attrsNS) {
        if (!attrsNS) return;
        for (const key in attrsNS) {
            el.setAttributeNS(attrsNS[key].ns, key, attrsNS[key].v);
        }
    }

    function applyStyles(el, styles) {
        if (!styles) return;
        for (const key in styles) {
            el.style.setProperty(key, styles[key]);
        }
    }

    function deferSelectValue(el, val) {
        requestAnimationFrame(() => { el.value = val; });
    }

    // =========================================================================
    // 9. Plugin registration (global API)
    // =========================================================================

    ns.register = function(name, handler) {
        ns._plugins[name] = handler;
    };

    // =========================================================================
    // Dynamic mount API — mount a component into an arbitrary DOM element
    // =========================================================================

    // ns.mount(name, element) mounts the named component into the given element.
    // Sets g-component attribute if not already present and sends a
    // BROWSER_INIT_REQUEST to the server.
    ns.mount = function(name, element) {
        if (!name || !element) return;
        if (!element.getAttribute("g-component")) {
            element.setAttribute("g-component", name);
        }
        sendInitRequest(name);
    };

    // =========================================================================
    // ExecJS — Go → browser → Go request/response
    // =========================================================================

    function handleJSCall(msg) {
        const id = msg.callId;
        const expr = msg.expr;
        let result = null;
        let error = "";

        if (window.GODOM_DISABLE_EXEC) {
            sendJSResult(id, new Uint8Array(0), "ExecJS is disabled on this browser");
            return;
        }

        try {
            const val = (0, eval)(expr); // indirect eval — global scope
            const json = JSON.stringify(val);
            if (json === undefined) {
                // Value is non-serializable (undefined, function, symbol)
                result = new Uint8Array(0);
                error = "non-serializable value";
            } else {
                result = textEncoder.encode(json);
            }
        } catch (e) {
            result = new Uint8Array(0);
            error = e.message || String(e);
        }
        sendJSResult(id, result, error);
    }

    function sendJSResult(id, result, error) {
        const msg = Proto.BrowserMessage.encode({
            kind: BK.BROWSER_JSRESULT,
            callId: id,
            result: result,
            error: error
        }).finish();
        ws.send(msg);
    }

    // =========================================================================
    // godom.call — JS → Go method calls from arbitrary JavaScript
    // =========================================================================

    // ns.call(method, ...args) sends a MethodCall to Go.
    // The method is dispatched to the component that owns the calling context.
    // For now, uses nodeId=0 (server resolves to the first component).
    ns.call = function(method) {
        const args = [];
        for (let i = 1; i < arguments.length; i++) {
            const json = JSON.stringify(arguments[i]);
            if (json !== undefined) {
                args.push(textEncoder.encode(json));
            }
        }
        const msg = Proto.BrowserMessage.encode({
            kind: BK.BROWSER_METHOD,
            nodeId: 0,
            method: method,
            args: args
        }).finish();
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(msg);
        }
    };

    // =========================================================================
    // Boot
    // =========================================================================

    connect();
})();
