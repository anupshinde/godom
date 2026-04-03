package main

// TreeData is the data struct sent to the tree plugin via g-plugin:tree.
type TreeData struct {
	Items       []TreeItem
	SelectedID  string
	ExpandedIDs []string
}

func (d *TreeData) addExpanded(id string) {
	for _, eid := range d.ExpandedIDs {
		if eid == id {
			return
		}
	}
	d.ExpandedIDs = append(d.ExpandedIDs, id)
}

func (d *TreeData) removeExpanded(id string) {
	for i, eid := range d.ExpandedIDs {
		if eid == id {
			d.ExpandedIDs = append(d.ExpandedIDs[:i], d.ExpandedIDs[i+1:]...)
			return
		}
	}
}

// TreeItem is a single item in the tree, matching what the plugin JS expects.
type TreeItem struct {
	ID       string
	Name     string
	Children []TreeItem
}

// categoriesToTreeData converts Category slice to plugin tree data.
func categoriesToTreeData(cats []Category) TreeData {
	return TreeData{Items: categoriesToItems(cats)}
}

func categoriesToItems(cats []Category) []TreeItem {
	items := make([]TreeItem, len(cats))
	for i, c := range cats {
		items[i] = TreeItem{
			ID:       c.ID,
			Name:     c.Name,
			Children: categoriesToItems(c.Children),
		}
	}
	return items
}
