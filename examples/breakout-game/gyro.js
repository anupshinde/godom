// Gyroscope bridge: reads device orientation and syncs tilt angle
// to a hidden <input id="gyro-input" g-bind="Tilt"> via input events.
// Reads mode from <span id="gyro-mode"> to pick the correct axis.
(function() {
    var pending = false;
    var lastVal = "";
    var listening = false;

    function onOrientation(e) {
        var gamma = e.gamma;
        var beta = e.beta;
        if (gamma == null || beta == null) return;

        // Read current mode from DOM (set by godom via g-text)
        var modeEl = document.getElementById("gyro-mode");
        var mode = modeEl ? modeEl.textContent.trim() : "off";
        if (mode === "off") return;

        var tilt;
        if (mode === "landscape") {
            // Phone on its side — beta is the left/right axis.
            // Sign depends on which side is down; use gamma sign to pick.
            tilt = gamma > 0 ? -beta : beta;
        } else {
            // Portrait — gamma is left/right
            tilt = gamma;
        }

        var val = tilt.toFixed(1);
        if (val === lastVal) return;
        lastVal = val;

        if (!pending) {
            pending = true;
            requestAnimationFrame(function() {
                pending = false;
                var input = document.getElementById("gyro-input");
                if (input) {
                    input.value = lastVal;
                    input.dispatchEvent(new Event("input", {bubbles: true}));
                }
            });
        }
    }

    function startListening() {
        if (listening) return;
        listening = true;
        window.addEventListener("deviceorientation", onOrientation);
    }

    // Touch slider: drag the circle on the tilt bar to control paddle
    function sendTilt(val) {
        var input = document.getElementById("gyro-input");
        if (input) {
            input.value = val;
            input.dispatchEvent(new Event("input", {bubbles: true}));
        }
    }

    function handleTiltTouch(e) {
        var bar = e.currentTarget;
        var rect = bar.getBoundingClientRect();
        var touch = e.touches[0];
        var ratio = (touch.clientX - rect.left) / rect.width;
        ratio = Math.max(0, Math.min(1, ratio));
        // Map 0..1 to tilt range: landscape ±20, portrait ±45
        var modeEl = document.getElementById("gyro-mode");
        var mode = modeEl ? modeEl.textContent.trim() : "off";
        var maxTilt = mode === "landscape" ? 20 : 45;
        var tilt = (ratio * 2 - 1) * maxTilt;
        sendTilt(tilt.toFixed(1));
    }

    // Poll for tilt-bar element and attach touch listeners
    var barPollCount = 0;
    var barPoll = setInterval(function() {
        var bar = document.querySelector(".tilt-bar");
        if (bar) {
            clearInterval(barPoll);
            bar.addEventListener("touchstart", function(e) {
                e.preventDefault();
                handleTiltTouch(e);
            }, {passive: false});
            bar.addEventListener("touchmove", function(e) {
                e.preventDefault();
                handleTiltTouch(e);
            }, {passive: false});
        }
        if (++barPollCount > 50) clearInterval(barPoll);
    }, 100);

    if (typeof DeviceOrientationEvent !== "undefined" &&
        typeof DeviceOrientationEvent.requestPermission === "function") {
        document.addEventListener("touchstart", function once() {
            document.removeEventListener("touchstart", once);
            DeviceOrientationEvent.requestPermission().then(function(state) {
                if (state === "granted") startListening();
            });
        }, {once: true});
    } else {
        startListening();
    }
})();
