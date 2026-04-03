// Shoelace tree bridge plugin for godom.
// Renders sl-tree/sl-tree-item from Go data, syncs selection and
// expand/collapse state via godom.call back to Go.
(function() {
    var godom = window[window.GODOM_NS || 'godom'];
    var suppressEvents = false; // prevent feedback loops during state sync

    godom.register("tree", {
        init: function(el, data) {
            el._treeEl = document.createElement("sl-tree");
            el._treeEl.setAttribute("selection", "single");
            renderItems(el._treeEl, data.Items || []);
            el.appendChild(el._treeEl);

            // Apply initial state
            applyState(el._treeEl, data);

            // Selection change → Go
            el._treeEl.addEventListener("sl-selection-change", function(e) {
                if (suppressEvents) return;
                var selected = e.detail.selection;
                if (selected && selected.length > 0) {
                    var id = selected[0].getAttribute("data-id") || "";
                    if (id) {
                        godom.call("SelectCategory", id);
                    }
                }
            });

            // Expand → Go
            el._treeEl.addEventListener("sl-expand", function(e) {
                if (suppressEvents) return;
                var item = e.target;
                if (item && item.getAttribute) {
                    var id = item.getAttribute("data-id");
                    if (id) godom.call("ExpandCategory", id);
                }
            });

            // Collapse → Go
            el._treeEl.addEventListener("sl-collapse", function(e) {
                if (suppressEvents) return;
                var item = e.target;
                if (item && item.getAttribute) {
                    var id = item.getAttribute("data-id");
                    if (id) godom.call("CollapseCategory", id);
                }
            });
        },

        update: function(el, data) {
            if (!el._treeEl) return;
            // Only apply state — don't rebuild the tree unless items changed.
            applyState(el._treeEl, data);
        }
    });

    function applyState(treeEl, data) {
        suppressEvents = true;
        try {
            var items = treeEl.querySelectorAll("sl-tree-item");
            var expandedSet = {};
            var expanded = data.ExpandedIDs || [];
            for (var i = 0; i < expanded.length; i++) {
                expandedSet[expanded[i]] = true;
            }

            for (var i = 0; i < items.length; i++) {
                var item = items[i];
                var id = item.getAttribute("data-id") || "";

                // Sync expanded state
                if (expandedSet[id]) {
                    if (!item.expanded) item.expanded = true;
                } else {
                    if (item.expanded) item.expanded = false;
                }

                // Sync selected state
                if (id === data.SelectedID) {
                    if (!item.selected) item.selected = true;
                } else {
                    if (item.selected) item.selected = false;
                }
            }
        } finally {
            suppressEvents = false;
        }
    }

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
