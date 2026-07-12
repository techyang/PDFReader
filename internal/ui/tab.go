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

	outline      []pdfengine.OutlineNode
	outlineModel *outlineModel

	tabPage     *walk.TabPage
	pageScroll  *walk.ScrollView
	pageView    *walk.CustomWidget
	outlineTree *walk.TreeView

	// continuousLayout/continuousLayoutW/continuousTotalH cache the last
	// computed continuous-mode page layout (see ensureContinuousLayout in
	// pageview.go). continuousLayout is nil whenever it needs
	// recomputing (mode just turned on, zoom changed, or never computed).
	continuousLayout  []document.PageLayout
	continuousLayoutW float64
	continuousTotalH  float64

	searchMatches []pdfengine.SearchMatch
	searchIndex   int // index into searchMatches of the currently-highlighted match

	searchBar  *walk.Composite
	searchEdit *walk.LineEdit
}

func newTab(path string, doc *pdfengine.Document) *tab {
	return &tab{
		path:  path,
		doc:   doc,
		page:  0,
		zoom:  document.Zoom{Mode: document.ZoomFitPage},
		cache: document.NewCache(16),
	}
}
