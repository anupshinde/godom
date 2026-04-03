package main

// treePluginJS is the inline Shoelace tree bridge plugin.
// It renders sl-tree/sl-tree-item from Go data and calls
// godom.call("SelectCategory", id) on selection change.
var treePluginJS = `
(function() {
    var godom = window[window.GODOM_NS || 'godom'];
    godom.register("tree", {
        init: function(el, data) {
            el._treeEl = document.createElement("sl-tree");
            el._treeEl.setAttribute("selection", "single");
            renderItems(el._treeEl, data.Items || []);
            el.appendChild(el._treeEl);

            el._treeEl.addEventListener("sl-selection-change", function(e) {
                var selected = e.detail.selection;
                if (selected && selected.length > 0) {
                    var id = selected[0].getAttribute("data-id") || "";
                    if (id) {
                        godom.call("SelectCategory", id);
                    }
                }
            });
        },
        update: function(el, data) {
            if (el._treeEl) {
                el._treeEl.innerHTML = "";
                renderItems(el._treeEl, data.Items || []);
            }
        }
    });

    function renderItems(parent, items) {
        for (var i = 0; i < items.length; i++) {
            var item = items[i];
            var el = document.createElement("sl-tree-item");
            el.setAttribute("data-id", item.ID || "");
            el.textContent = item.Name || "";
            if (item.Children && item.Children.length > 0) {
                renderItems(el, item.Children);
            }
            parent.appendChild(el);
        }
    }
})();
`
