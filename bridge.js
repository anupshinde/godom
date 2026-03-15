(function() {
    var ws;
    var gidMap = {};    // data-gid → DOM element cache
    var anchorMap = {}; // g-for id → {start, end} comment nodes
    var eventMap = {};  // "gid:event" → latest event config (for dedup)
    var pluginState = {}; // gid → true if init has been called

    function connect() {
        ws = new WebSocket("ws://" + location.host + "/ws");

        ws.onmessage = function(evt) {
            var msg = JSON.parse(evt.data);
            if (msg.type === "init") {
                gidMap = {};
                anchorMap = {};
                eventMap = {};
                indexDOM(document.body);
                execCommands(msg.commands);
                registerEvents(msg.events);
            } else if (msg.type === "update") {
                execCommands(msg.commands);
            }
        };

        ws.onclose = function() {
            setTimeout(connect, 1000);
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
                case "re-event":
                    registerEvents([c.val]);
                    break;
                default:
                    var el = getEl(c.id);
                    if (!el) break;
                    switch (c.op) {
                        case "text":
                            el.textContent = c.val != null ? String(c.val) : "";
                            break;
                        case "value":
                            var sv = c.val != null ? String(c.val) : "";
                            if (el.value !== sv) el.value = sv;
                            break;
                        case "checked":
                            el.checked = !!c.val;
                            break;
                        case "display":
                            el.style.display = c.val ? "" : "none";
                            break;
                        case "class":
                            if (c.val) el.classList.add(c.name);
                            else el.classList.remove(c.name);
                            break;
                        case "attr":
                            el.setAttribute(c.name, c.val != null ? String(c.val) : "");
                            break;
                        case "plugin":
                            var handler = window.godom && window.godom._plugins && window.godom._plugins[c.name];
                            if (handler) {
                                if (!pluginState[c.id]) {
                                    handler.init(el, c.val);
                                    pluginState[c.id] = true;
                                } else {
                                    handler.update(el, c.val);
                                }
                            }
                            break;
                    }
            }
        }
    }

    function execList(c) {
        var a = anchorMap[c.id];
        if (!a || !a.start || !a.end) return;
        var start = a.start, end = a.end;

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
            var tmp = document.createElement("div");
            tmp.innerHTML = item.html;
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

        for (var i = 0; i < c.items.length; i++) {
            var item = c.items[i];
            var tmp = document.createElement("div");
            tmp.innerHTML = item.html;
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
        var count = c.val;

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

    function registerEvents(evts) {
        if (!evts) return;
        for (var i = 0; i < evts.length; i++) {
            var e = evts[i];
            var el = getEl(e.id);
            if (!el) continue;

            var key = e.id + ":" + e.on;
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
                        var m = JSON.parse(JSON.stringify(ev.msg));
                        m.value = elem.value;
                        ws.send(JSON.stringify(m));
                    });
                })(key, el);
            } else if (e.on === "keydown") {
                (function(k, elem) {
                    elem.addEventListener("keydown", function(ke) {
                        var ev = eventMap[k];
                        if (!ev) return;
                        if (ev.key && ke.key !== ev.key) return;
                        ws.send(JSON.stringify(ev.msg));
                    });
                })(key, el);
            } else {
                (function(k, elem) {
                    elem.addEventListener(e.on, function() {
                        var ev = eventMap[k];
                        if (!ev) return;
                        ws.send(JSON.stringify(ev.msg));
                    });
                })(key, el);
            }
        }
    }

    connect();
})();
