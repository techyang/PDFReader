// internal/print/layout.go
package print

// ScaleMode controls how a rendered page bitmap is scaled onto the
// physical printable area when it doesn't exactly match the paper size.
type ScaleMode int

const (
	ScaleFitPage    ScaleMode = iota // scale to fill the page, aspect preserved
	ScaleActualSize                  // 1 PDF point = 1/72 inch on paper, no user scaling
	ScalePercent                    // like ScaleActualSize, then multiplied by Settings.ScalePercent/100
)

// Orientation mirrors DEVMODE's DMORIENT_* values, defined here (instead
// of importing lxn/win into this platform-agnostic file) purely so
// job.go/layout.go don't need a Windows-only dependency for something
// this simple.
type Orientation int

const (
	OrientPortrait Orientation = iota
	OrientLandscape
)

// destRect computes where on the printable canvas (device pixels, origin
// at the printable area's top-left, size canvasW x canvasH) a rendered
// page image of size imgW x imgH pixels should be drawn.
//
// widthPt/heightPt are the PDF page's real size in points (1/72 inch,
// from pdfengine.Document.PageSize) and dpiX/dpiY are the PRINTER's own
// device DPI (GetDeviceCaps LOGPIXELSX/Y) - both are only used by
// ScaleActualSize/ScalePercent to compute the page's true physical size
// on paper, independent of whatever DPI the source image was rendered
// at (a fixed 300 DPI regardless of the printer's real resolution, set
// elsewhere). ScaleFitPage ignores widthPt/heightPt/dpiX/dpiY entirely:
// the image already has the right aspect ratio, so it's fit to
// canvasW x canvasH directly.
//
// The result is always centered within the canvas and clamped so x/y
// are never negative, even if the requested size exceeds the canvas
// (e.g. actual-size on a page bigger than the paper) - mirrors the same
// clamp-at-0 centering already used for on-screen single-page painting
// in pageview.go's paintTab.
func destRect(imgW, imgH int, widthPt, heightPt float64, dpiX, dpiY, canvasW, canvasH int32, mode ScaleMode, percent int) (x, y, w, h int32) {
	if imgW <= 0 || imgH <= 0 {
		return 0, 0, 0, 0
	}
	switch mode {
	case ScaleActualSize, ScalePercent:
		factor := 1.0
		if mode == ScalePercent {
			factor = float64(percent) / 100.0
		}
		w = int32(widthPt / 72 * float64(dpiX) * factor)
		h = int32(heightPt / 72 * float64(dpiY) * factor)
	default: // ScaleFitPage
		scale := float64(canvasW) / float64(imgW)
		if alt := float64(canvasH) / float64(imgH); alt < scale {
			scale = alt
		}
		w = int32(float64(imgW) * scale)
		h = int32(float64(imgH) * scale)
	}

	x = (canvasW - w) / 2
	y = (canvasH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y, w, h
}
