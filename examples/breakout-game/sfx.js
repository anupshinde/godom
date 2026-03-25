// Sound effects and vibration for Breakout.
// Watches a hidden <span id="sound-event"> for changes via MutationObserver.
// Uses Web Audio API to generate tones — no audio files needed.
(function() {
    var audioCtx = null;

    function getCtx() {
        if (!audioCtx) {
            audioCtx = new (window.AudioContext || window.webkitAudioContext)();
        }
        if (audioCtx.state === "suspended") {
            audioCtx.resume();
        }
        return audioCtx;
    }

    // Pre-warm AudioContext on first user gesture so it's ready for sounds
    document.addEventListener("click", function warmUp() {
        document.removeEventListener("click", warmUp);
        getCtx();
    }, {once: true});

    function beep(freq, duration, volume, type) {
        try {
            var ctx = getCtx();
            var osc = ctx.createOscillator();
            var gain = ctx.createGain();
            osc.type = type || "square";
            osc.frequency.value = freq;
            gain.gain.value = volume || 0.1;
            gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + duration);
            osc.connect(gain);
            gain.connect(ctx.destination);
            osc.start(ctx.currentTime);
            osc.stop(ctx.currentTime + duration);
        } catch(e) {}
    }

    function vibrate(ms) {
        if (navigator.vibrate) {
            navigator.vibrate(ms);
        }
    }

    var sounds = {
        brick: function() { beep(520, 0.08, 0.08, "square"); },
        paddle: function() { beep(300, 0.1, 0.1, "triangle"); },
        wall: function() { beep(200, 0.05, 0.05, "sine"); },
        life: function() {
            beep(400, 0.15, 0.12, "square");
            setTimeout(function() { beep(300, 0.15, 0.12, "square"); }, 150);
            setTimeout(function() { beep(200, 0.3, 0.12, "square"); }, 300);
            vibrate([100, 50, 200]);
        },
        gameover: function() {
            beep(300, 0.2, 0.12, "square");
            setTimeout(function() { beep(250, 0.2, 0.12, "square"); }, 200);
            setTimeout(function() { beep(200, 0.2, 0.12, "square"); }, 400);
            setTimeout(function() { beep(150, 0.5, 0.12, "sawtooth"); }, 600);
            vibrate([200, 100, 200, 100, 400]);
        },
        win: function() {
            beep(523, 0.12, 0.1, "square");
            setTimeout(function() { beep(659, 0.12, 0.1, "square"); }, 120);
            setTimeout(function() { beep(784, 0.12, 0.1, "square"); }, 240);
            setTimeout(function() { beep(1047, 0.3, 0.12, "square"); }, 360);
            vibrate([50, 50, 50, 50, 200]);
        }
    };

    function isControllerVisible() {
        var ctrl = document.querySelector(".controller");
        return ctrl && getComputedStyle(ctrl).display !== "none";
    }

    // If this tab shows the controller, announce it via hidden input → Go field.
    // Game-view tabs read the synced flag to decide whether to play sounds.
    function announceController() {
        if (!isControllerVisible()) return;
        var input = document.getElementById("controller-input");
        if (input && input.value !== "1") {
            input.value = "1";
            input.dispatchEvent(new Event("input", {bubbles: true}));
        }
    }

    function shouldPlaySounds() {
        // Controller tab: always play
        if (isControllerVisible()) return true;
        // Game-view tab: play only if no controller is active
        var flag = document.getElementById("controller-flag");
        var controllerActive = flag && flag.textContent.trim() === "1";
        return !controllerActive;
    }

    // Poll for the sound-event element (built async by godom bridge)
    var pollCount = 0;
    var poll = setInterval(function() {
        var el = document.getElementById("sound-event");
        if (el) {
            clearInterval(poll);
            announceController();
            var observer = new MutationObserver(function() {
                if (!shouldPlaySounds()) return;
                var evt = el.textContent.trim();
                if (evt && sounds[evt]) {
                    sounds[evt]();
                }
            });
            observer.observe(el, { childList: true, characterData: true, subtree: true });
        }
        if (++pollCount > 50) clearInterval(poll);
    }, 100);
})();
