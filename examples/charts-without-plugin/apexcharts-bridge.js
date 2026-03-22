godom.register("apexcharts", {
    init: function(el, data) {
        var chart = new ApexCharts(el, data);
        el.__chart = chart;
        el.__chartReady = chart.render();
    },
    update: function(el, data) {
        if (!el.__chart) return;
        var chart = el.__chart;
        var ready = el.__chartReady || Promise.resolve();
        ready.then(function() {
            chart.updateSeries(data.series, false);
        });
    }
});
