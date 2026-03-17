(function() {
    var ws;
    var gidMap = {};    // data-gid → DOM element cache
    var anchorMap = {}; // g-for id → {start, end} comment nodes
    var eventMap = {};  // "gid:event" → latest event config (for dedup)
    var pluginState = {}; // gid → true if init has been called

    var Proto = godomProto; // defined in protocol.js
    var textEncoder = new TextEncoder();
    var textDecoder = new TextDecoder();

    // Disconnect overlay — shown when the WebSocket connection is lost
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

    function connect() {
        ws = new WebSocket("ws://" + location.host + "/ws");
        ws.binaryType = "arraybuffer";

        ws.onmessage = function(evt) {
            var msg = Proto.ServerMessage.decode(new Uint8Array(evt.data));
            if (msg.type === "init") {
                hideDisconnectOverlay();
                gidMap = {};
                anchorMap = {};
                eventMap = {};
                indexDOM(document.body);
                execCommands(msg.commands);
                registerEvents(msg.events);
            } else if (msg.type === "update") {
                execCommands(msg.commands);
                if (msg.events && msg.events.length) registerEvents(msg.events);
            }
        };

        ws.onclose = function(evt) {
            var errorMsg = evt.reason || null;
            showDisconnectOverlay(errorMsg);
            if (!errorMsg) setTimeout(connect, 1000);
        };

        ws.onerror = function() {
            ws.close();
        };
    }

    // Build gid→element map and anchor map by walking the DOM once
    function indexDOM(root) {
        var all = root.querySelectorAll("[data-gid]");
        for (var i = 0; i < all.length; i++) {
            gidMap[all[i].getAttribute("data-gid")] = all[i];
        }
        // Index g-for anchor comments
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

    function getEl(gid) {
        var el = gidMap[gid];
        if (el) return el;
        // Fallback: query DOM (handles race or missing index)
        el = document.querySelector("[data-gid=\"" + gid + "\"]");
        if (el) gidMap[gid] = el;
        return el;
    }

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
                        case "text":
                            el.textContent = c.strVal || "";
                            break;
                        case "value":
                            var sv = c.strVal || "";
                            if (el.value !== sv) el.value = sv;
                            break;
                        case "checked":
                            el.checked = !!c.boolVal;
                            break;
                        case "display":
                            el.style.display = c.boolVal ? "" : "none";
                            break;
                        case "class":
                            if (c.boolVal) el.classList.add(c.name);
                            else el.classList.remove(c.name);
                            break;
                        case "attr":
                            el.setAttribute(c.name, c.strVal || "");
                            break;
                        case "style":
                            el.style.setProperty(c.name, c.strVal || "");
                            break;
                        case "draggable":
                            el.setAttribute("draggable", "true");
                            el.dataset.gDrag = c.strVal || "";
                            el.dataset.gDragGroup = c.name || "";
                            if (!el.getAttribute("data-g-dragstart")) {
                                el.setAttribute("data-g-dragstart", "1");
                                el.addEventListener("dragstart", function(de) {
                                    de.dataTransfer.effectAllowed = "move";
                                    var dragEl = de.target.closest("[data-g-drag]");
                                    var group = dragEl.dataset.gDragGroup || "";
                                    de.dataTransfer.setData("application/x-godom-" + group, dragEl.dataset.gDrag);
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
                            break;
                        case "dropzone":
                            el.dataset.gDrop = c.strVal || "";
                            break;
                        case "plugin":
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
                            break;
                    }
            }
        }
    }

    // Index anchor comments within a node (for nested g-for support)
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

    // Context-sensitive HTML parsing: certain elements (tr, td, option, etc.)
    // are stripped by the browser when parsed via innerHTML on a <div>.
    // Use the parent element's tag to determine the correct wrapper.
    var contextWrappers = {
        "TABLE":    function() { return document.createElement("table"); },
        "THEAD":    function() { var t = document.createElement("table"); var s = document.createElement("thead"); t.appendChild(s); return s; },
        "TBODY":    function() { var t = document.createElement("table"); var s = document.createElement("tbody"); t.appendChild(s); return s; },
        "TFOOT":    function() { var t = document.createElement("table"); var s = document.createElement("tfoot"); t.appendChild(s); return s; },
        "TR":       function() { var t = document.createElement("table"); var b = document.createElement("tbody"); var r = document.createElement("tr"); t.appendChild(b); b.appendChild(r); return r; },
        "SELECT":   function() { return document.createElement("select"); },
        "OPTGROUP": function() { var s = document.createElement("select"); var g = document.createElement("optgroup"); s.appendChild(g); return g; }
    };

    function createTmpContainer(html, parentTag) {
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

    function execList(c) {
        var a = anchorMap[c.id];
        if (!a || !a.start || !a.end) return;
        var start = a.start, end = a.end;
        var parentTag = start.parentNode.tagName;

        // Remove old items between anchors and clear their gidMap entries
        while (start.nextSibling !== end) {
            var removing = start.nextSibling;
            // Clear cached gid entries for removed nodes
            if (removing.nodeType === 1) {
                var gid = removing.getAttribute("data-gid");
                if (gid) delete gidMap[gid];
                var subs = removing.querySelectorAll("[data-gid]");
                for (var j = 0; j < subs.length; j++) {
                    delete gidMap[subs[j].getAttribute("data-gid")];
                }
            }
            start.parentNode.removeChild(removing);
        }

        // Insert new items and index them
        for (var i = 0; i < c.items.length; i++) {
            var item = c.items[i];
            var tmp = createTmpContainer(item.html, parentTag);
            while (tmp.firstChild) {
                var node = tmp.firstChild;
                start.parentNode.insertBefore(node, end);
                // Index new node
                if (node.nodeType === 1) {
                    var ng = node.getAttribute("data-gid");
                    if (ng) gidMap[ng] = node;
                    var newSubs = node.querySelectorAll("[data-gid]");
                    for (var k = 0; k < newSubs.length; k++) {
                        gidMap[newSubs[k].getAttribute("data-gid")] = newSubs[k];
                    }
                    indexAnchors(node);
                }
            }
            execCommands(item.cmds);
            registerEvents(item.evts);
        }
    }

    function execListAppend(c) {
        var a = anchorMap[c.id];
        if (!a || !a.end) return;
        var end = a.end;
        var parentTag = end.parentNode.tagName;

        for (var i = 0; i < c.items.length; i++) {
            var item = c.items[i];
            var tmp = createTmpContainer(item.html, parentTag);
            while (tmp.firstChild) {
                var node = tmp.firstChild;
                end.parentNode.insertBefore(node, end);
                if (node.nodeType === 1) {
                    var ng = node.getAttribute("data-gid");
                    if (ng) gidMap[ng] = node;
                    var subs = node.querySelectorAll("[data-gid]");
                    for (var k = 0; k < subs.length; k++) {
                        gidMap[subs[k].getAttribute("data-gid")] = subs[k];
                    }
                    indexAnchors(node);
                }
            }
            execCommands(item.cmds);
            registerEvents(item.evts);
        }
    }

    function execListTruncate(c) {
        var a = anchorMap[c.id];
        if (!a || !a.end) return;
        var end = a.end;
        var count = c.numVal;

        for (var i = 0; i < count; i++) {
            var prev = end.previousSibling;
            if (!prev || prev === a.start) break;
            // Clear gidMap entries
            if (prev.nodeType === 1) {
                var gid = prev.getAttribute("data-gid");
                if (gid) delete gidMap[gid];
                var subs = prev.querySelectorAll("[data-gid]");
                for (var j = 0; j < subs.length; j++) {
                    delete gidMap[subs[j].getAttribute("data-gid")];
                }
            }
            prev.parentNode.removeChild(prev);
        }
    }

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

            // For keydown with key filter, include the key in the map key
            // so multiple bindings (ArrowUp:Up, ArrowDown:Down) coexist.
            var key = e.id + ":" + e.on;
            if (e.on === "keydown" && e.key) {
                key += ":" + e.key;
            }
            eventMap[key] = e; // store/update latest config

            // Only add the listener once per gid+event pair.
            // The listener reads from eventMap at fire time, so
            // re-events just update the map — no duplicate listeners.
            if (el.getAttribute("data-evt-" + e.on)) continue;
            el.setAttribute("data-evt-" + e.on, "1");

            if (e.on === "input") {
                (function(k, elem) {
                    elem.addEventListener("input", function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        var valBytes = textEncoder.encode(JSON.stringify(elem.value));
                        sendEnvelope(ev.msg, null, valBytes);
                    });
                })(key, el);
            } else if (e.on === "keydown") {
                (function(gid, elem) {
                    elem.addEventListener("keydown", function(ke) {
                        // Look up by key-specific entry first, then unfiltered
                        var ev = eventMap[gid + ":keydown:" + ke.key]
                              || eventMap[gid + ":keydown"];
                        if (!ev) return;
                        sendEnvelope(ev.msg);
                    });
                })(e.id, el);
            } else if (e.on === "mousedown" || e.on === "mouseup") {
                (function(k, elem, evtName) {
                    elem.addEventListener(evtName, function(me) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [me.offsetX, me.offsetY]);
                    });
                })(key, el, e.on);
            } else if (e.on === "mousemove") {
                (function(k, elem) {
                    var pending = null;
                    var scheduled = false;
                    elem.addEventListener("mousemove", function(me) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        // Throttle: only send once per animation frame
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
            } else if (e.on === "wheel") {
                (function(k, elem) {
                    elem.addEventListener("wheel", function(we) {
                        we.preventDefault();
                        var ev = eventMap[k];
                        if (!ev) return;
                        sendEnvelope(ev.msg, [we.deltaY]);
                    }, {passive: false});
                })(key, el);
            } else if (e.on === "drop") {
                (function(k, elem, group) {
                    var mimeType = "application/x-godom-" + group;
                    var dragCounter = 0;
                    function hasMatch(dt) {
                        for (var t = 0; t < dt.types.length; t++) {
                            if (dt.types[t] === mimeType) return true;
                        }
                        return false;
                    }
                    elem.addEventListener("dragover", function(de) {
                        if (!hasMatch(de.dataTransfer)) return;
                        de.preventDefault();
                        de.dataTransfer.dropEffect = "move";
                        var rect = elem.getBoundingClientRect();
                        var isAbove = de.clientY < rect.top + rect.height / 2;
                        elem.classList.remove("g-drag-over-above", "g-drag-over-below");
                        elem.classList.add("g-drag-over");
                        elem.classList.add(isAbove ? "g-drag-over-above" : "g-drag-over-below");
                    });
                    elem.addEventListener("dragenter", function(de) {
                        if (!hasMatch(de.dataTransfer)) return;
                        de.preventDefault();
                        dragCounter++;
                    });
                    elem.addEventListener("dragleave", function() {
                        if (dragCounter === 0) return;
                        dragCounter--;
                        if (dragCounter === 0) {
                            elem.classList.remove("g-drag-over", "g-drag-over-above", "g-drag-over-below");
                        }
                    });
                    elem.addEventListener("drop", function(de) {
                        dragCounter = 0;
                        elem.classList.remove("g-drag-over", "g-drag-over-above", "g-drag-over-below");
                        if (!hasMatch(de.dataTransfer)) return;
                        de.preventDefault();
                        var ev = eventMap[k];
                        if (!ev) return;
                        var fromStr = de.dataTransfer.getData(mimeType);
                        var toStr = elem.dataset.gDrop || elem.dataset.gDrag || "";
                        var rect = elem.getBoundingClientRect();
                        var position = de.clientY < rect.top + rect.height / 2 ? "above" : "below";
                        function smartVal(s) { var n = Number(s); return s !== "" && !isNaN(n) ? n : s; }
                        var dropArgs = [smartVal(fromStr), smartVal(toStr), position];
                        sendEnvelope(ev.msg, null, textEncoder.encode(JSON.stringify(dropArgs)));
                    });
                })(key, el, e.key || "");
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

    connect();
})();
