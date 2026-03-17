// bridge.js — godom's browser-side command executor.
//
// This is the thin layer between Go and the DOM. Go sends binary protobuf
// messages over WebSocket; this script decodes them and applies concrete
// DOM operations (setText, setAttr, appendHTML, etc.). It does NOT evaluate
// expressions, diff state, or make rendering decisions — Go does all of that.
//
// Structure:
//   1. State & globals     — gidMap, anchorMap, eventMap, pluginState
//   2. Connection          — WebSocket connect/reconnect, disconnect overlay
//   3. DOM indexing        — build gid→element and anchor maps from the DOM
//   4. Command execution   — apply DOM commands from Go (text, attr, class, list, etc.)
//   5. List operations     — g-for rendering: full replace, append, truncate
//   6. Event registration  — wire up DOM events that send messages back to Go
//   7. Drag & drop         — draggable/dropzone setup with group filtering

(function() {

    // =========================================================================
    // 1. State & globals
    // =========================================================================

    var ws;
    var gidMap = {};    // data-gid → DOM element cache
    var anchorMap = {}; // g-for id → {start, end} comment nodes that mark list boundaries (<!-- g-for:id --> ... <!-- /g-for:id -->)
    var eventMap = {};  // "gid:event" → latest event config (for dedup)
    var pluginState = {}; // gid → true if plugin init has been called

    var Proto = godomProto; // protobuf definitions, loaded from protocol.js
    var textEncoder = new TextEncoder();
    var textDecoder = new TextDecoder();

    // =========================================================================
    // 2. Connection — WebSocket with auto-reconnect and disconnect overlay
    // =========================================================================

    var overlay = null;

    // Show a fullscreen overlay when disconnected or crashed.
    // If errorMsg is provided, it's a crash — show the panic message, no reconnect.
    // If null, it's a normal disconnect — show "Waiting for server..." and retry.
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

    // Reconnect delay with exponential backoff: 1s, 2s, 4s, 8s, ... up to 30s.
    // Reset to 1s on successful connection.
    var reconnectDelay = 1000;

    // Connect to Go via WebSocket. On "init" messages, rebuild all indexes
    // and apply the full command set. On "update" messages, apply incremental
    // commands. On close, show overlay and auto-reconnect (unless it was a crash).
    function connect() {
        // Use wss:// when behind a TLS-terminating reverse proxy (nginx, Caddy, etc.)
        var wsProto = location.protocol === "https:" ? "wss://" : "ws://";
        ws = new WebSocket(wsProto + location.host + "/ws");
        ws.binaryType = "arraybuffer";

        ws.onmessage = function(evt) {
            var msg = Proto.ServerMessage.decode(new Uint8Array(evt.data));
            if (msg.type === "init") {
                // Full page init — clear all caches and rebuild from DOM
                hideDisconnectOverlay();
                reconnectDelay = 1000; // reset backoff on successful connection
                gidMap = {};
                anchorMap = {};
                eventMap = {};
                pluginState = {};
                indexDOM(document.body);
                execCommands(msg.commands);
                registerEvents(msg.events);
            } else if (msg.type === "update") {
                // Incremental update — apply only changed commands/events
                execCommands(msg.commands);
                if (msg.events && msg.events.length) registerEvents(msg.events);
            }
        };

        // Convention: Go server sets evt.reason to the panic message on crash,
        // and closes without a reason on normal shutdown.
        ws.onclose = function(evt) {
            var errorMsg = evt.reason || null;
            showDisconnectOverlay(errorMsg);
            // Auto-reconnect on normal disconnect, not on crash.
            // Uses exponential backoff: 1s, 2s, 4s, ... capped at 30s.
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
    // 3. DOM indexing — map data-gid attributes and g-for comment anchors
    // =========================================================================

    // Walk the DOM once to build two maps:
    // - gidMap: data-gid attribute value → DOM element (for fast lookups)
    // - anchorMap: g-for id → {start, end} comment nodes (for list rendering)
    //
    // g-for lists are bounded by comment pairs:
    //   <!-- g-for:listId --> ... items ... <!-- /g-for:listId -->
    function indexDOM(root) {
        var all = root.querySelectorAll("[data-gid]");
        for (var i = 0; i < all.length; i++) {
            gidMap[all[i].getAttribute("data-gid")] = all[i];
        }
        var walker = document.createTreeWalker(
            root, NodeFilter.SHOW_COMMENT, null, false
        );
        while (walker.nextNode()) {
            var text = walker.currentNode.nodeValue.trim();
            var m;
            if ((m = text.match(/^g-for:(.+)$/))) {
                if (!anchorMap[m[1]]) anchorMap[m[1]] = {};
                anchorMap[m[1]].start = walker.currentNode;
            } else if ((m = text.match(/^\/g-for:(.+)$/))) {
                if (!anchorMap[m[1]]) anchorMap[m[1]] = {};
                anchorMap[m[1]].end = walker.currentNode;
            }
        }
    }

    // Index anchor comments within a subtree (called after inserting new
    // g-for items that may themselves contain nested g-for loops).
    function indexAnchors(node) {
        if (node.nodeType !== 1) return;
        var walker = document.createTreeWalker(node, NodeFilter.SHOW_COMMENT, null, false);
        while (walker.nextNode()) {
            var text = walker.currentNode.nodeValue.trim();
            var m;
            if ((m = text.match(/^g-for:(.+)$/))) {
                if (!anchorMap[m[1]]) anchorMap[m[1]] = {};
                anchorMap[m[1]].start = walker.currentNode;
            } else if ((m = text.match(/^\/g-for:(.+)$/))) {
                if (!anchorMap[m[1]]) anchorMap[m[1]] = {};
                anchorMap[m[1]].end = walker.currentNode;
            }
        }
    }

    // Look up a DOM element by its gid. Uses the cache first, falls back
    // to a querySelector if the element was added after initial indexing.
    function getEl(gid) {
        var el = gidMap[gid];
        if (el) return el;
        // Fallback: query the DOM directly. If this fires, it likely means
        // indexDOM or insertAndIndex missed this element — worth investigating.
        console.warn("[godom] gidMap miss for", gid, "— falling back to querySelector");
        el = document.querySelector("[data-gid=\"" + gid + "\"]");
        if (el) gidMap[gid] = el;
        return el;
    }

    // =========================================================================
    // 4. Command execution — dispatch DOM operations from Go
    // =========================================================================

    // Each command has an op (operation type), an id (target element's gid),
    // and payload fields (strVal, boolVal, name, numVal, rawVal, items).
    // List ops are dispatched separately; all others resolve the target element first.
    function execCommands(cmds) {
        if (!cmds) return;
        for (var i = 0; i < cmds.length; i++) {
            var c = cmds[i];
            switch (c.op) {
                case "list":
                    execList(c);
                    break;
                case "list-append":
                    execListAppend(c);
                    break;
                case "list-truncate":
                    execListTruncate(c);
                    break;
                default:
                    var el = getEl(c.id);
                    if (!el) break;
                    switch (c.op) {
                        case "text":      // g-text: set text content
                            el.textContent = c.strVal || "";
                            break;
                        case "value":     // g-bind: set input value (skip if unchanged to preserve cursor)
                            var sv = c.strVal || "";
                            if (el.value !== sv) el.value = sv;
                            break;
                        case "checked":   // g-checked: set checkbox state
                            el.checked = !!c.boolVal;
                            break;
                        case "display":   // g-show/g-if: toggle visibility
                            el.style.display = c.boolVal ? "" : "none";
                            break;
                        case "class":     // g-class:name: add/remove CSS class
                            if (c.boolVal) el.classList.add(c.name);
                            else el.classList.remove(c.name);
                            break;
                        case "attr":      // g-attr:name: set HTML/SVG attribute
                            el.setAttribute(c.name, c.strVal || "");
                            break;
                        case "style":     // g-style:prop: set inline style property
                            el.style.setProperty(c.name, c.strVal || "");
                            break;
                        case "draggable": // g-draggable: make element draggable (see §7)
                            setupDraggable(el, c);
                            break;
                        case "dropzone":  // g-dropzone: mark as drop target
                            el.dataset.gDrop = c.strVal || "";
                            break;
                        case "plugin":    // g-plugin:name: send data to JS plugin
                            execPlugin(el, c);
                            break;
                    }
            }
        }
    }

    // =========================================================================
    // 5. List operations — g-for rendering
    // =========================================================================
    //
    // Lists are rendered between comment anchor pairs:
    //   <!-- g-for:id --> ... DOM nodes ... <!-- /g-for:id -->
    //
    // Three operations:
    //   list          — full replace: remove all old items, insert new ones
    //   list-append   — add new items at the end (optimization for growing lists)
    //   list-truncate — remove N items from the end (optimization for shrinking lists)
    //
    // Each item carries: html (raw HTML string), cmds (commands to apply after
    // insertion), and evts (events to register on the new elements).

    // Context-sensitive HTML parsing: certain elements (tr, td, option, etc.)
    // are stripped by the browser when parsed via innerHTML on a <div>.
    // Use the parent element's tag to determine the correct wrapper.
    //
    // TABLE is special: unlike other parents, the correct wrapper depends on
    // what's being inserted, not just where. A <table> can contain <thead>,
    // <tbody>, <tr>, <colgroup>, etc. — and <tr> needs a <tbody> wrapper to
    // prevent browser auto-insertion, while <td>/<th> need <tbody><tr>.
    // Other table children (thead, tbody, tfoot, colgroup, caption) parse
    // correctly inside a plain <table>. So TABLE inspects the HTML to pick
    // the right wrapper — this is the one case where string inspection is
    // justified over pure parent-tag lookup.
    var contextWrappers = {
        "THEAD":    function() { var t = document.createElement("table"); var s = document.createElement("thead"); t.appendChild(s); return s; },
        "TBODY":    function() { var t = document.createElement("table"); var s = document.createElement("tbody"); t.appendChild(s); return s; },
        "TFOOT":    function() { var t = document.createElement("table"); var s = document.createElement("tfoot"); t.appendChild(s); return s; },
        "TR":       function() { var t = document.createElement("table"); var b = document.createElement("tbody"); var r = document.createElement("tr"); t.appendChild(b); b.appendChild(r); return r; },
        "COLGROUP": function() { var t = document.createElement("table"); var c = document.createElement("colgroup"); t.appendChild(c); return c; },
        "SELECT":   function() { return document.createElement("select"); },
        "OPTGROUP": function() { var s = document.createElement("select"); var g = document.createElement("optgroup"); s.appendChild(g); return g; }
    };

    // Parse an HTML string into DOM nodes using the correct parent context.
    // Returns a container element whose children are the parsed nodes.
    function createTmpContainer(html, parentTag) {
        // TABLE is ambiguous — inspect the child tag to pick the right wrapper
        if (parentTag === "TABLE") {
            var m = /^\s*<\s*([a-z0-9]+)/i.exec(html);
            var firstTag = m ? m[1].toUpperCase() : "";

            if (firstTag === "TR") {
                // <tr> needs <tbody> to prevent browser auto-insertion
                var t = document.createElement("table");
                var b = document.createElement("tbody");
                t.appendChild(b);
                b.innerHTML = html;
                return b;
            }

            if (firstTag === "TD" || firstTag === "TH") {
                // <td>/<th> are only valid under <tr>, not directly under <tbody>
                var t2 = document.createElement("table");
                var b2 = document.createElement("tbody");
                var r = document.createElement("tr");
                t2.appendChild(b2);
                b2.appendChild(r);
                r.innerHTML = html;
                return r;
            }

            // All other table children (thead, tbody, tfoot, colgroup, caption)
            // parse correctly inside a plain <table>
            var table = document.createElement("table");
            table.innerHTML = html;
            return table;
        }

        var factory = contextWrappers[parentTag];
        if (factory) {
            var container = factory();
            container.innerHTML = html;
            return container;
        }
        var div = document.createElement("div");
        div.innerHTML = html;
        return div;
    }

    // Move all child nodes from the temporary container into the real DOM
    // (before `beforeNode`), and register each inserted element in gidMap.
    // Also scans for nested g-for anchor comments so inner loops work.
    // Note: tmp.firstChild is used as the loop condition because insertBefore
    // moves the node out of tmp, advancing firstChild automatically.
    function insertAndIndex(tmp, parent, beforeNode) {
        while (tmp.firstChild) {
            var node = tmp.firstChild;
            parent.insertBefore(node, beforeNode);
            if (node.nodeType === 1) { // element nodes only (skip text/comment)
                var ng = node.getAttribute("data-gid");
                if (ng) gidMap[ng] = node;
                // Also index any gid elements nested inside this node
                var subs = node.querySelectorAll("[data-gid]");
                for (var k = 0; k < subs.length; k++) {
                    gidMap[subs[k].getAttribute("data-gid")] = subs[k];
                }
                // item.html always inserts as one root element, so nested g-for anchors
                // (if any) are always inside `node`, never top-level siblings here.
                indexAnchors(node);
            }
        }
    }

    // Remove eventMap entries for a given gid. Keys are "gid:event" or
    // "gid:keydown:key", so we match any key starting with "gid:".
    function cleanEventMap(gid) {
        var prefix = gid + ":";
        for (var k in eventMap) {
            if (k.indexOf(prefix) === 0) delete eventMap[k];
        }
    }

    // Clean up an anchorMap entry if this comment node is a g-for boundary.
    // Deletes the entire entry when both start and end are gone.
    function cleanAnchorComment(commentNode) {
        var text = commentNode.nodeValue.trim();
        var m;
        if ((m = text.match(/^g-for:(.+)$/))) {
            if (anchorMap[m[1]] && anchorMap[m[1]].start === commentNode) {
                delete anchorMap[m[1]].start;
                if (!anchorMap[m[1]].end) delete anchorMap[m[1]];
            }
        } else if ((m = text.match(/^\/g-for:(.+)$/))) {
            if (anchorMap[m[1]] && anchorMap[m[1]].end === commentNode) {
                delete anchorMap[m[1]].end;
                if (!anchorMap[m[1]].start) delete anchorMap[m[1]];
            }
        }
    }

    // Remove a DOM node and clean up all gidMap, pluginState, eventMap, and
    // anchorMap entries for it and its descendants. This prevents stale references
    // to removed/detached nodes and ensures correct re-initialization if ids are reused.
    function removeAndClean(node) {
        if (node.nodeType === 1) {
            var gid = node.getAttribute("data-gid");
            if (gid) {
                delete gidMap[gid];
                delete pluginState[gid];
                cleanEventMap(gid);
            }
            var subs = node.querySelectorAll("[data-gid]");
            for (var j = 0; j < subs.length; j++) {
                var subGid = subs[j].getAttribute("data-gid");
                delete gidMap[subGid];
                delete pluginState[subGid];
                cleanEventMap(subGid);
            }
            // Clean up any nested g-for anchor comments
            var walker = document.createTreeWalker(node, NodeFilter.SHOW_COMMENT, null, false);
            while (walker.nextNode()) {
                cleanAnchorComment(walker.currentNode);
            }
        } else if (node.nodeType === 8) { // comment node removed directly
            cleanAnchorComment(node);
        }
        node.parentNode.removeChild(node);
    }

    // Full list replace: remove all items between anchors, insert new ones.
    function execList(c) {
        var a = anchorMap[c.id];
        if (!a || !a.start || !a.end) return;
        var start = a.start, end = a.end;
        var parentTag = start.parentNode.tagName;

        // Remove all existing items between the start and end anchors
        while (start.nextSibling !== end) {
            removeAndClean(start.nextSibling);
        }

        // Insert new items
        for (var i = 0; i < c.items.length; i++) {
            var item = c.items[i];
            var tmp = createTmpContainer(item.html, parentTag);
            insertAndIndex(tmp, start.parentNode, end);
            execCommands(item.cmds);
            registerEvents(item.evts);
        }
    }

    // Append new items to the end of a list (before the end anchor).
    function execListAppend(c) {
        var a = anchorMap[c.id];
        if (!a || !a.end) return;
        var end = a.end;
        var parentTag = end.parentNode.tagName;

        for (var i = 0; i < c.items.length; i++) {
            var item = c.items[i];
            var tmp = createTmpContainer(item.html, parentTag);
            insertAndIndex(tmp, end.parentNode, end);
            execCommands(item.cmds);
            registerEvents(item.evts);
        }
    }

    // Remove N items from the end of a list (working backwards from end anchor).
    //
    // list-truncate relies on a structural invariant:
    // every g-for item renders to exactly one top-level DOM element.
    // item.html is produced by html.Render on the single element that had g-for,
    // so removing one sibling per count is correct.
    function execListTruncate(c) {
        var a = anchorMap[c.id];
        if (!a || !a.end) return;
        var end = a.end;
        var count = c.numVal;

        for (var i = 0; i < count; i++) {
            var prev = end.previousSibling;
            if (!prev || prev === a.start) break;
            removeAndClean(prev);
        }
    }

    // =========================================================================
    // 6. Event registration — wire DOM events to send messages back to Go
    // =========================================================================
    //
    // Events use a dedup strategy: eventMap stores the latest config for each
    // gid+event pair. The actual DOM listener is added only once; it reads
    // from eventMap at fire time. When Go re-sends events (e.g. after a g-for
    // re-render), the map is updated but no duplicate listeners are created.

    // Encode and send a message to Go via the WebSocket as a protobuf Envelope.
    //   msg   — the method/handler name in Go (e.g. "Increment", "Toggle(3)")
    //   args  — optional numeric args array (e.g. [x, y] for mouse events)
    //   value — optional raw bytes (e.g. JSON-encoded input value or drop data)
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
            var el = getEl(e.id);
            if (!el) continue;

            // Build a unique key for this event binding.
            // Keydown events include the key so multiple bindings coexist
            // (e.g. ArrowUp:MoveUp and ArrowDown:MoveDown on the same element).
            var key = e.id + ":" + e.on;
            if (e.on === "keydown" && e.key) {
                key += ":" + e.key;
            }
            eventMap[key] = e;

            // Skip if we already added a listener for this event type on this element.
            // The listener reads from eventMap, so updating the map is sufficient.
            if (el.getAttribute("data-evt-" + e.on)) continue;
            el.setAttribute("data-evt-" + e.on, "1");

            // Each event type below uses an IIFE — (function(k, elem) { ... })(key, el) —
            // to capture the current values of `key` and `el` in a closure. Without this,
            // all listeners would share the same loop variable and only reference the last
            // element. This is standard pre-ES6 JS; `let` would make it unnecessary.

            // --- g-bind: two-way input binding ---
            // Sends the current input value to Go on every keystroke.
            if (e.on === "input") {
                (function(k, elem) {
                    elem.addEventListener("input", function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        var valBytes = textEncoder.encode(JSON.stringify(elem.value));
                        sendEnvelope(ev.msg, null, valBytes);
                    });
                })(key, el);

            // --- g-keydown: key-filtered keyboard events ---
            // Looks up key-specific binding first, falls back to unfiltered.
            } else if (e.on === "keydown") {
                (function(gid, elem) {
                    elem.addEventListener("keydown", function(ke) {
                        var ev = eventMap[gid + ":keydown:" + ke.key]
                              || eventMap[gid + ":keydown"];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(e.id, el);

            // --- g-mousedown / g-mouseup: sends (x, y) offset coordinates ---
            } else if (e.on === "mousedown" || e.on === "mouseup") {
                (function(k, elem, evtName) {
                    elem.addEventListener(evtName, function(me) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [me.offsetX, me.offsetY]);
                    });
                })(key, el, e.on);

            // --- g-mousemove: throttled to one send per animation frame ---
            } else if (e.on === "mousemove") {
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

            // --- g-wheel: scroll events, sends deltaY ---
            } else if (e.on === "wheel") {
                (function(k, elem) {
                    elem.addEventListener("wheel", function(we) {
                        we.preventDefault();
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [we.deltaY]);
                    }, {passive: false});
                })(key, el);

            // --- g-drop: drag-and-drop with group filtering (see §7) ---
            } else if (e.on === "drop") {
                setupDropHandler(key, el, e.key || "");

            // --- g-click and other simple events: no payload ---
            } else {
                (function(k, elem) {
                    elem.addEventListener(e.on, function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(key, el);
            }
        }
    }

    // =========================================================================
    // 7. Drag & drop
    // =========================================================================

    // Set up an element as draggable. Stores drag data and group in dataset
    // attributes. Listeners are added once (guarded by data-g-dragstart flag).
    function setupDraggable(el, c) {
        el.setAttribute("draggable", "true");
        el.dataset.gDrag = c.strVal || "";
        el.dataset.gDragGroup = c.name || "";
        if (!el.getAttribute("data-g-dragstart")) {
            el.setAttribute("data-g-dragstart", "1");
            // On drag start: store drag data in a group-namespaced MIME type
            el.addEventListener("dragstart", function(de) {
                de.dataTransfer.effectAllowed = "move";
                var dragEl = de.target.closest("[data-g-drag]");
                var group = dragEl.dataset.gDragGroup || "";
                de.dataTransfer.setData("application/x-godom-" + group, dragEl.dataset.gDrag);
                dragEl.classList.add("g-dragging");
            });
            // On drag end: clean up all drag CSS classes
            el.addEventListener("dragend", function(de) {
                de.target.closest("[data-g-drag]").classList.remove("g-dragging");
                var overs = document.querySelectorAll(".g-drag-over,.g-drag-over-above,.g-drag-over-below");
                for (var j = 0; j < overs.length; j++) {
                    overs[j].classList.remove("g-drag-over", "g-drag-over-above", "g-drag-over-below");
                }
            });
        }
    }

    // Set up drop handling on an element. Uses a group-namespaced MIME type
    // to filter which draggables are accepted. Tracks enter/leave counts to
    // handle nested elements correctly.
    //
    // On drop, sends [from, to, position] to Go where:
    //   from     = the draggable's data value
    //   to       = the dropzone's data value (or the target's drag data)
    //   position = "above" or "below" based on cursor position within the element
    function setupDropHandler(key, el, group) {
        var mimeType = "application/x-godom-" + group;
        var dragCounter = 0;

        // Check if the drag event carries our group's MIME type
        function hasMatch(dt) {
            for (var t = 0; t < dt.types.length; t++) {
                if (dt.types[t] === mimeType) return true;
            }
            return false;
        }

        // Allow drop and show position indicator (above/below)
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

        // Track drag enter/leave for nested element handling
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

        // Handle the drop: extract data, determine position, send to Go
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
            // Preserve numeric types: "3" → 3, "hello" → "hello"
            function smartVal(s) { var n = Number(s); return s !== "" && !isNaN(n) ? n : s; }
            var dropArgs = [smartVal(fromStr), smartVal(toStr), position];
            sendEnvelope(ev.msg, null, textEncoder.encode(JSON.stringify(dropArgs)));
        });
    }

    // Execute a plugin command. Calls init() on first use, update() after.
    // Plugin data is JSON-encoded in rawVal.
    function execPlugin(el, c) {
        var handler = window.godom && window.godom._plugins && window.godom._plugins[c.name];
        if (handler) {
            var data = c.rawVal && c.rawVal.length ? JSON.parse(textDecoder.decode(c.rawVal)) : null;
            if (!pluginState[c.id]) {
                handler.init(el, data);
                pluginState[c.id] = true;
            } else {
                handler.update(el, data);
            }
        }
    }

    // =========================================================================
    // Boot
    // =========================================================================

    connect();
})();
