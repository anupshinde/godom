godom.register("plotly", {
    init: function(el, data) {
        Plotly.newPlot(el, data.data || [], data.layout || {}, data.config || {});
    },
    update: function(el, data) {
        Plotly.react(el, data.data || [], data.layout || {}, data.config || {});
    }
});
