godom.register("canvas3d", {
    init: function(el, data) {
        el.width = data.width || 900;
        el.height = data.height || 600;
        el.__ctx = el.getContext("2d");
        this._draw(el.__ctx, data);
    },
    update: function(el, data) {
        var ctx = el.__ctx;
        if (!ctx) return;
        this._draw(ctx, data);
    },
    _draw: function(ctx, data) {
        var w = ctx.canvas.width, h = ctx.canvas.height;
        // Black background with subtle stars
        ctx.fillStyle = "#050510";
        ctx.fillRect(0, 0, w, h);

        var cmds = data.commands || [];
        for (var i = 0; i < cmds.length; i++) {
            var c = cmds[i];
            ctx.save();

            if (c.glow) {
                // Sun glow effect
                var grd = ctx.createRadialGradient(c.x, c.y, 0, c.x, c.y, c.r * 3);
                grd.addColorStop(0, c.color);
                grd.addColorStop(0.3, c.color);
                grd.addColorStop(1, "transparent");
                ctx.fillStyle = grd;
                ctx.beginPath();
                ctx.arc(c.x, c.y, c.r * 3, 0, Math.PI * 2);
                ctx.fill();

                // Sun body
                ctx.fillStyle = c.color;
                ctx.beginPath();
                ctx.arc(c.x, c.y, c.r, 0, Math.PI * 2);
                ctx.fill();
            } else {
                // Planet with shading
                var b = c.brightness || 1;
                ctx.globalAlpha = 0.3 + 0.7 * b;

                if (c.ring) {
                    // Draw ring behind/around planet
                    ctx.strokeStyle = c.ringColor || "#D4BE8D";
                    ctx.lineWidth = 3;
                    ctx.globalAlpha = 0.5;
                    ctx.beginPath();
                    ctx.ellipse(c.x, c.y, c.r * 2.2, c.r * 0.6, 0, 0, Math.PI * 2);
                    ctx.stroke();
                    ctx.globalAlpha = 0.3 + 0.7 * b;
                }

                // Planet body with gradient for 3D look
                var pgrd = ctx.createRadialGradient(
                    c.x - c.r * 0.3, c.y - c.r * 0.3, c.r * 0.1,
                    c.x, c.y, c.r
                );
                pgrd.addColorStop(0, c.color);
                pgrd.addColorStop(1, _darken(c.color, 0.4));
                ctx.fillStyle = pgrd;
                ctx.beginPath();
                ctx.arc(c.x, c.y, c.r, 0, Math.PI * 2);
                ctx.fill();
            }

            ctx.restore();
        }
    }
});

function _darken(hex, factor) {
    // Simple hex color darkening
    var r = parseInt(hex.slice(1, 3), 16);
    var g = parseInt(hex.slice(3, 5), 16);
    var b = parseInt(hex.slice(5, 7), 16);
    r = Math.floor(r * factor);
    g = Math.floor(g * factor);
    b = Math.floor(b * factor);
    return "#" + ((1 << 24) + (r << 16) + (g << 8) + b).toString(16).slice(1);
}
