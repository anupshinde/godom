package main

// TreeData is the data struct sent to the tree plugin via g-plugin:tree.
type TreeData struct {
	Items []TreeItem
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
