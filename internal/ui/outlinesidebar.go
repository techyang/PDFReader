// internal/ui/outlinesidebar.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/pdfengine"
)

// outlineItem adapts one pdfengine.OutlineNode to walk.TreeItem.
type outlineItem struct {
	node     pdfengine.OutlineNode
	parent   *outlineItem
	children []*outlineItem
}

func newOutlineItem(node pdfengine.OutlineNode, parent *outlineItem) *outlineItem {
	item := &outlineItem{node: node, parent: parent}
	item.children = make([]*outlineItem, len(node.Children))
	for i, child := range node.Children {
		item.children[i] = newOutlineItem(child, item)
	}
	return item
}

func (i *outlineItem) Text() string                    { return i.node.Title }
func (i *outlineItem) ChildCount() int                 { return len(i.children) }
func (i *outlineItem) ChildAt(index int) walk.TreeItem { return i.children[index] }
func (i *outlineItem) Parent() walk.TreeItem {
	if i.parent == nil {
		return nil
	}
	return i.parent
}

// outlineModel implements walk.TreeModel over a document's outline.
type outlineModel struct {
	walk.TreeModelBase
	roots []*outlineItem
}

func newOutlineModel(nodes []pdfengine.OutlineNode) *outlineModel {
	m := &outlineModel{roots: make([]*outlineItem, len(nodes))}
	for i, n := range nodes {
		m.roots[i] = newOutlineItem(n, nil)
	}
	return m
}

func (m *outlineModel) RootCount() int                 { return len(m.roots) }
func (m *outlineModel) RootAt(index int) walk.TreeItem { return m.roots[index] }

// findByPage returns the first outline item (depth-first, root order)
// whose PageIndex equals page, or nil if none match. Used to sync the
// outline tree's selection to whichever page is currently most visible
// while scrolling in continuous reading mode.
func (m *outlineModel) findByPage(page int) *outlineItem {
	for _, root := range m.roots {
		if item := root.findByPage(page); item != nil {
			return item
		}
	}
	return nil
}

func (i *outlineItem) findByPage(page int) *outlineItem {
	if i.node.PageIndex == page {
		return i
	}
	for _, child := range i.children {
		if item := child.findByPage(page); item != nil {
			return item
		}
	}
	return nil
}
