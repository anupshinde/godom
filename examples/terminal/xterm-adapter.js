godom.register("xterm", {
    init: function(el, data) {
        var term = new Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, monospace",
            theme: {
                background: "#1a1a2e",
                foreground: "#e0e0e0",
                cursor: "#e94560",
                selectionBackground: "rgba(233, 69, 96, 0.3)"
            }
        });

        var fitAddon = new FitAddon.FitAddon();
        term.loadAddon(fitAddon);
        term.open(el);
        fitAddon.fit();

        // Connect to the terminal WebSocket for raw PTY I/O.
        var wsUrl = "ws://" + location.hostname + ":" + data.wsPort + "/terminal?token=" + encodeURIComponent(data.token);
        var ws = new WebSocket(wsUrl);
        ws.binaryType = "arraybuffer";

        ws.onopen = function() {
            // Send initial terminal size.
            ws.send(JSON.stringify({ cols: term.cols, rows: term.rows }));
            term.focus();
        };

        ws.onmessage = function(evt) {
            term.write(new Uint8Array(evt.data));
        };

        ws.onclose = function() {
            term.write("\r\n\x1b[1;31m[Disconnected]\x1b[0m\r\n");
        };

        // Send keystrokes to Go.
        term.onData(function(input) {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(new TextEncoder().encode(input));
            }
        });

        // Handle resize.
        term.onResize(function(size) {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ cols: size.cols, rows: size.rows }));
            }
        });

        window.addEventListener("resize", function() {
            fitAddon.fit();
        });

        el.__term = term;
        el.__ws = ws;
        el.__fitAddon = fitAddon;
    },
    update: function(el, data) {
        // Terminal config is static after init — nothing to update.
    }
});
