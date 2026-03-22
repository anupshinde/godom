godom.register("videocanvas", {
    init: function(el, data) {
        el.width = data.width || 960;
        el.height = data.height || 540;
        el.__ctx = el.getContext("2d");
        // Black initial frame
        var ctx = el.__ctx;
        ctx.fillStyle = "#000";
        ctx.fillRect(0, 0, el.width, el.height);
        if (data.frame) {
            this._drawFrame(el, data);
        }
    },
    update: function(el, data) {
        if (!el.__ctx || !data.frame) return;
        this._drawFrame(el, data);
    },
    _drawFrame: function(el, data) {
        var ctx = el.__ctx;
        var img = new Image();
        img.onload = function() {
            ctx.drawImage(img, 0, 0, el.width, el.height);
        };
        img.src = "data:image/jpeg;base64," + data.frame;
    }
});
