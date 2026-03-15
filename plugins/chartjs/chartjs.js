godom.register("chartjs", {
    init: function(el, data) {
        el.__chart = new Chart(el, {
            type: data.type,
            data: { labels: data.labels || [], datasets: data.datasets || [] },
            options: data.options || {}
        });
    },
    update: function(el, data) {
        var chart = el.__chart;
        if (!chart) return;
        chart.data.labels = data.labels || [];
        var ds = data.datasets || [];
        for (var i = 0; i < ds.length; i++) {
            if (chart.data.datasets[i]) {
                chart.data.datasets[i].data = ds[i].data || [];
            }
        }
        chart.update("none");
    }
});
