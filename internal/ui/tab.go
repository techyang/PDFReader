// internal/ui/tab.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/document"
	"pdfreader/internal/pdfengine"
)

// tab holds all runtime state for one open document.
type tab struct {
	path string
	doc  *pdfengine.Document

	page  int // 0-based current page index
	zoom  document.Zoom
	cache *document.Cache

	outline []pdfengine.OutlineNode

	tabPage     *walk.TabPage
	pageView    *walk.CustomWidget
	outlineTree *walk.TreeView
}

func newTab(path string, doc *pdfengine.Document) *tab {
	return &tab{
		path:  path,
		doc:   doc,
		page:  0,
		zoom:  document.Zoom{Mode: document.ZoomFitPage},
		cache: document.NewCache(5),
	}
}
