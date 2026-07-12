package document

// PageLayout describes one page's position within the full scrollable
// content of a continuously-scrolled document, at a given zoom level.
type PageLayout struct {
	Top    float64 // px, top offset within the full virtual content
	Width  float64 // px
	Height float64 // px
	DPI    int     // dpi to render this page at (matches pdfengine.RenderPage's dpi param)
}

// LayoutContinuous computes per-page layout for continuous-scroll mode.
// pageSizesPt is each page's (widthPt, heightPt) in document order. zoom
// must not be ZoomFitPage - callers are responsible for downgrading to
// ZoomFitWidth first, since fit-page has no meaning when many pages are
// visible at once (passing 0 as the viewport height below makes a
// ZoomFitPage caller's mistake produce an obviously-wrong degenerate
// layout instead of a plausible-looking wrong one). viewportWidthPx is
// used for ZoomFitWidth/ZoomPercent scale calculation. gapPx is the
// vertical spacing drawn between consecutive pages.
func LayoutContinuous(pageSizesPt [][2]float64, zoom Zoom, viewportWidthPx, gapPx float64) ([]PageLayout, float64) {
	layouts := make([]PageLayout, len(pageSizesPt))
	top := 0.0
	for i, size := range pageSizesPt {
		widthPt, heightPt := size[0], size[1]
		scale := zoom.ScaleFactor(widthPt, heightPt, viewportWidthPx, 0)
		dpi := DPIForScale(scale)
		widthPx := widthPt / 72.0 * float64(dpi)
		heightPx := heightPt / 72.0 * float64(dpi)

		layouts[i] = PageLayout{Top: top, Width: widthPx, Height: heightPx, DPI: dpi}
		top += heightPx + gapPx
	}

	total := top
	if len(layouts) > 0 {
		total -= gapPx // no trailing gap after the last page
	}
	return layouts, total
}

// VisiblePages returns the [start,end) index range (into layouts) of
// pages that intersect the vertical range [scrollTop, scrollTop+viewportHeight).
// Pages that only touch the range at an edge (no interior overlap) don't
// count. Returns (0, 0) if layouts is empty or nothing intersects.
func VisiblePages(layouts []PageLayout, scrollTop, viewportHeight float64) (start, end int) {
	scrollBottom := scrollTop + viewportHeight
	start = -1
	for i, l := range layouts {
		pageBottom := l.Top + l.Height
		if pageBottom <= scrollTop {
			continue
		}
		if l.Top >= scrollBottom {
			break
		}
		if start == -1 {
			start = i
		}
		end = i + 1
	}
	if start == -1 {
		return 0, 0
	}
	return start, end
}

// MostVisiblePage returns the index into layouts of the page covering
// the largest visible area within [scrollTop, scrollTop+viewportHeight).
// Returns 0 if layouts is empty. Ties resolve to the earlier (smaller
// index) page.
func MostVisiblePage(layouts []PageLayout, scrollTop, viewportHeight float64) int {
	if len(layouts) == 0 {
		return 0
	}
	scrollBottom := scrollTop + viewportHeight
	best := 0
	bestVisible := -1.0
	for i, l := range layouts {
		pageBottom := l.Top + l.Height
		visibleTop := l.Top
		if scrollTop > visibleTop {
			visibleTop = scrollTop
		}
		visibleBottom := pageBottom
		if scrollBottom < visibleBottom {
			visibleBottom = scrollBottom
		}
		visible := visibleBottom - visibleTop
		if visible > bestVisible {
			bestVisible = visible
			best = i
		}
	}
	return best
}
