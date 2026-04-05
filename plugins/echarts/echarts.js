godom.register("echarts", {
    init: function(el, data) {
        var chart = echarts.init(el);
        el.__echart = chart;
        chart.setOption(data);
        // Handle resize when container size changes.
        new ResizeObserver(function() { chart.resize(); }).observe(el);
    },
    update: function(el, data) {
        var chart = el.__echart;
        if (!chart) return;
        chart.setOption(data, { notMerge: false });
    }
});
