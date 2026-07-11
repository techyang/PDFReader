// internal/ui/pageview.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/document"
)

// renderCurrentPage renders t's current page at t's current zoom (using
// the cache when possible) and returns a walk.Bitmap ready to paint.
// The caller owns the returned bitmap and must Dispose() it when replaced.
func (t *tab) renderCurrentPage(viewportW, viewportH float64) (*walk.Bitmap, error) {
	pageWidthPt, pageHeightPt := 612.0, 792.0 // US Letter fallback; refined in Task 14 via real page size.

	scale := t.zoom.ScaleFactor(pageWidthPt, pageHeightPt, viewportW, viewportH)
	dpi := document.DPIForScale(scale)

	key := document.CacheKey{Page: t.page, DPI: dpi}
	img, ok := t.cache.Get(key)
	if !ok {
		rendered, err := t.doc.RenderPage(t.page, dpi)
		if err != nil {
			return nil, err
		}
		img = rendered
		t.cache.Put(key, img)
	}

	return walk.NewBitmapFromImage(img)
}
