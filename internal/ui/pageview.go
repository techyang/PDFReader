// internal/ui/pageview.go
package ui

import (
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"

	"pdfreader/internal/document"
	"pdfreader/internal/pdfengine"
)

const continuousPageGapPx = 8

// renderPageBitmap renders page at dpi (using cache when possible) and
// returns a walk.Bitmap ready to paint. The caller owns the returned
// bitmap and must Dispose() it when done.
func renderPageBitmap(doc *pdfengine.Document, cache *document.Cache, page, dpi int) (*walk.Bitmap, error) {
	key := document.CacheKey{Page: page, DPI: dpi}
	img, ok := cache.Get(key)
	if !ok {
		rendered, err := doc.RenderPage(page, dpi)
		if err != nil {
			return nil, err
		}
		img = rendered
		cache.Put(key, img)
	}
	return walk.NewBitmapFromImage(img)
}

// renderCurrentPage renders t's current page at t's current zoom (using
// the cache when possible) and returns a walk.Bitmap ready to paint.
// The caller owns the returned bitmap and must Dispose() it when replaced.
// Only used in single-page mode - continuous mode renders each visible
// page directly via renderPageBitmap (see paintContinuousTab in app.go).
func (t *tab) renderCurrentPage(viewportW, viewportH float64) (*walk.Bitmap, error) {
	pageWidthPt, pageHeightPt, err := t.doc.PageSize(t.page)
	if err != nil {
		return nil, err
	}

	scale := t.zoom.ScaleFactor(pageWidthPt, pageHeightPt, viewportW, viewportH)
	dpi := document.DPIForScale(scale)

	return renderPageBitmap(t.doc, t.cache, t.page, dpi)
}

// ensureContinuousLayout recomputes t's continuous-mode page layout if it
// hasn't been computed yet (t.continuousLayout == nil) or the viewport
// width has changed since the last computation (which changes
// ZoomFitWidth/ZoomPercent scale-to-pixel results), and resizes pageView
// to match the new total virtual content size so pageScroll's scrollbar
// range stays correct.
func ensureContinuousLayout(t *tab, viewportWidthPx float64) error {
	if t.continuousLayout != nil && t.continuousLayoutW == viewportWidthPx {
		return nil
	}

	sizes := make([][2]float64, t.doc.PageCount())
	for i := range sizes {
		w, h, err := t.doc.PageSize(i)
		if err != nil {
			return err
		}
		sizes[i] = [2]float64{w, h}
	}

	layouts, total := document.LayoutContinuous(sizes, t.zoom, viewportWidthPx, continuousPageGapPx)

	maxWidth := viewportWidthPx
	for _, l := range layouts {
		if l.Width > maxWidth {
			maxWidth = l.Width
		}
	}

	t.continuousLayout = layouts
	t.continuousLayoutW = viewportWidthPx
	t.continuousTotalH = total

	return resizePageView(t, walk.Size{Width: int(maxWidth), Height: int(total)})
}

// resizePageView sets pageView's size and mirrors it onto pageView's
// real native parent - not pageScroll itself, but pageScroll's internal
// composite (see continuousScrollY's comment below for why). walk
// normally keeps that composite sized to fit its content via
// scrollViewLayoutItem.PerformLayout (scrollview.go), but that only runs
// while pageScroll takes part in an ancestor's automatic layout pass -
// which it deliberately doesn't in this codebase (see the comment above
// content's creation in app.go, on why pageScroll's own bounds are set
// by hand instead). Left unmanaged, the composite stays stuck at its
// creation size - CW_USEDEFAULT resolves to 0x0 for a child window - so
// pageView ends up sized correctly but sitting inside a zero-area
// parent: Windows never considers it visible and never sends it
// WM_PAINT, no matter how many times it's resized or invalidated. This
// is the actual reason no PDF content ever rendered even after
// pageScroll itself started getting correct bounds.
func resizePageView(t *tab, size walk.Size) error {
	if parent, ok := t.pageView.Parent().(interface{ SetSizePixels(walk.Size) error }); ok {
		if err := parent.SetSizePixels(size); err != nil {
			return err
		}
	}
	return t.pageView.SetSizePixels(size)
}

// continuousScrollY returns how far pageView has been scrolled down
// within its ScrollView, in native pixels.
//
// walk.ScrollView (see its scrollview.go) implements scrolling by moving
// an internal, unexported content composite up/down via SetYPixels; the
// widgets added to the ScrollView (here, pageView) become native
// children of that composite (walk re-parents them there - see
// ContainerBase.onInsertedWidget in container.go). So pageView.Parent()
// returns that composite as a walk.Container, and the composite's own Y
// position (relative to its parent, the fixed-size ScrollView viewport)
// is always -scrollY. A local interface is enough to reach the public
// BoundsPixels method without needing the unexported concrete type.
func continuousScrollY(pageView *walk.CustomWidget) float64 {
	parent, ok := pageView.Parent().(interface{ BoundsPixels() walk.Rectangle })
	if !ok {
		return 0
	}
	return -float64(parent.BoundsPixels().Y)
}

// setContinuousScrollY scrolls pageScroll so pageView's content shows y
// (clamped to the valid range implied by totalHeight) at the top of the
// viewport, and keeps pageScroll's native scrollbar thumb in sync.
//
// walk.ScrollView has no public "scroll to" method - scroll() and
// updateScrollBars() in its scrollview.go are both unexported. Moving
// the composite directly via SetYPixels (see continuousScrollY above)
// would move the content but leave the native scrollbar thumb pointing
// at the old position, since only ScrollView's own WM_VSCROLL handler
// updates the win32 scrollbar's position. So this reaches into win32
// directly and does what that handler would do for a user-driven
// scroll: set the scrollbar's logical position, then move the content
// to match.
func setContinuousScrollY(pageScroll *walk.ScrollView, pageView *walk.CustomWidget, y, totalHeight float64) {
	viewportH := float64(pageScroll.ClientBoundsPixels().Height)
	maxY := totalHeight - viewportH
	if maxY < 0 {
		maxY = 0
	}
	if y < 0 {
		y = 0
	} else if y > maxY {
		y = maxY
	}

	parent, ok := pageView.Parent().(interface{ SetYPixels(int) error })
	if !ok {
		return
	}
	parent.SetYPixels(-int(y))

	si := win.SCROLLINFO{
		CbSize: uint32(unsafe.Sizeof(win.SCROLLINFO{})),
		FMask:  win.SIF_POS,
		NPos:   int32(y),
	}
	win.SetScrollInfo(pageScroll.Handle(), win.SB_VERT, &si, true)

	pageView.Invalidate()
}
