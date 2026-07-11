package document

import "math"

// ZoomMode selects how the page scale factor is derived.
type ZoomMode int

const (
	ZoomPercent ZoomMode = iota
	ZoomFitWidth
	ZoomFitPage
)

const (
	MinZoomPercent = 25.0
	MaxZoomPercent = 400.0
)

// Zoom is the current zoom setting for a tab.
type Zoom struct {
	Mode    ZoomMode
	Percent float64 // used when Mode == ZoomPercent, e.g. 100 for 100%.
}

// ScaleFactor returns the multiplier to apply to a page's 72-DPI point
// dimensions to get on-screen pixels, given the page size in points and
// the available viewport size in pixels.
func (z Zoom) ScaleFactor(pageWidthPt, pageHeightPt, viewportWidthPx, viewportHeightPx float64) float64 {
	switch z.Mode {
	case ZoomFitWidth:
		if pageWidthPt <= 0 {
			return 1.0
		}
		return viewportWidthPx / pageWidthPt
	case ZoomFitPage:
		if pageWidthPt <= 0 || pageHeightPt <= 0 {
			return 1.0
		}
		widthScale := viewportWidthPx / pageWidthPt
		heightScale := viewportHeightPx / pageHeightPt
		return math.Min(widthScale, heightScale)
	default: // ZoomPercent
		return z.Percent / 100.0
	}
}

// DPIForScale converts a scale factor (1.0 == 100%) to the DPI value to
// pass to pdfengine.RenderPage, using 72 DPI as the 100% baseline (PDF
// points are defined as 1/72 inch).
func DPIForScale(scale float64) int {
	return int(math.Round(72.0 * scale))
}

// ClampPercent clamps a percentage zoom value to [MinZoomPercent, MaxZoomPercent].
func ClampPercent(percent float64) float64 {
	if percent < MinZoomPercent {
		return MinZoomPercent
	}
	if percent > MaxZoomPercent {
		return MaxZoomPercent
	}
	return percent
}
