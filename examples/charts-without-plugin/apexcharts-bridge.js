godom.register("apexcharts", {
    init: function(el, data) {
        el.__chart = new ApexCharts(el, data);
        el.__chart.render();
    },
    update: function(el, data) {
        var chart = el.__chart;
        if (!chart) return;
        chart.updateSeries(data.series, false);
    }
});
