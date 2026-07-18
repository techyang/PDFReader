// internal/print/layout_test.go
package print

import "testing"

func TestDestRect_FitPage_PreservesAspectAndCenters(t *testing.T) {
	// 1000x2000 px image (portrait, taller than wide) into a 3000x3000 square canvas.
	x, y, w, h := destRect(1000, 2000, 0, 0, 300, 300, 3000, 3000, ScaleFitPage, 0)
	if w != 1500 || h != 3000 {
		t.Fatalf("w,h = %d,%d, want 1500,3000 (scaled by canvasH/imgH=1.5, capped by height)", w, h)
	}
	if x != 750 || y != 0 {
		t.Fatalf("x,y = %d,%d, want 750,0 (centered horizontally, flush vertically)", x, y)
	}
}

func TestDestRect_ActualSize_UsesPrinterDPI(t *testing.T) {
	// A 72pt x 144pt (1in x 2in) page at 300 DPI printer resolution should be
	// exactly 300x600 device px, regardless of the source image's own pixel size.
	x, y, w, h := destRect(300, 600, 72, 144, 300, 300, 3000, 3000, ScaleActualSize, 0)
	if w != 300 || h != 600 {
		t.Fatalf("w,h = %d,%d, want 300,600", w, h)
	}
	wantX, wantY := int32((3000-300)/2), int32((3000-600)/2)
	if x != wantX || y != wantY {
		t.Fatalf("x,y = %d,%d, want %d,%d (centered)", x, y, wantX, wantY)
	}
}

func TestDestRect_Percent_ScalesActualSize(t *testing.T) {
	// Same page as above, at 50% -> half the actual-size dimensions.
	_, _, w, h := destRect(300, 600, 72, 144, 300, 300, 3000, 3000, ScalePercent, 50)
	if w != 150 || h != 300 {
		t.Fatalf("w,h = %d,%d, want 150,300", w, h)
	}
}

func TestDestRect_NeverNegativeOffset(t *testing.T) {
	// Image bigger than the canvas (e.g. actual-size overflowing a small
	// canvas) must clamp x/y to 0, not go negative.
	x, y, _, _ := destRect(300, 600, 500, 1000, 300, 300, 100, 100, ScaleActualSize, 0)
	if x != 0 || y != 0 {
		t.Fatalf("x,y = %d,%d, want 0,0 (clamped)", x, y)
	}
}
