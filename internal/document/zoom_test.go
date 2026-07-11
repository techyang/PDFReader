package document

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestZoom_Percent(t *testing.T) {
	z := Zoom{Mode: ZoomPercent, Percent: 150}
	got := z.ScaleFactor(612, 792, 800, 600) // page size/viewport size irrelevant for ZoomPercent
	if !almostEqual(got, 1.5) {
		t.Fatalf("ScaleFactor = %v, want 1.5", got)
	}
}

func TestZoom_FitWidth(t *testing.T) {
	// Page is 200pt wide, viewport is 800px wide -> scale should make the
	// rendered page (at 72 DPI baseline) exactly fill the viewport width.
	z := Zoom{Mode: ZoomFitWidth}
	got := z.ScaleFactor(200, 400, 800, 600)
	want := 800.0 / 200.0
	if !almostEqual(got, want) {
		t.Fatalf("ScaleFactor = %v, want %v", got, want)
	}
}

func TestZoom_FitPage(t *testing.T) {
	// Page is 200x400pt, viewport is 800x600px. Fit-page must pick the
	// smaller of the width-fit and height-fit scales so the whole page
	// is visible.
	z := Zoom{Mode: ZoomFitPage}
	got := z.ScaleFactor(200, 400, 800, 600)
	widthScale := 800.0 / 200.0
	heightScale := 600.0 / 400.0
	want := math.Min(widthScale, heightScale)
	if !almostEqual(got, want) {
		t.Fatalf("ScaleFactor = %v, want %v", got, want)
	}
}

func TestZoom_DPIForScale(t *testing.T) {
	got := DPIForScale(1.0)
	if got != 72 {
		t.Fatalf("DPIForScale(1.0) = %d, want 72", got)
	}
	got = DPIForScale(2.0)
	if got != 144 {
		t.Fatalf("DPIForScale(2.0) = %d, want 144", got)
	}
}

func TestZoom_FitWidth_DegeneratePageWidth(t *testing.T) {
	z := Zoom{Mode: ZoomFitWidth}
	got := z.ScaleFactor(0, 400, 800, 600)
	if !almostEqual(got, 1.0) {
		t.Fatalf("ScaleFactor = %v, want 1.0 for degenerate page width", got)
	}
}

func TestZoom_FitPage_DegeneratePageSize(t *testing.T) {
	z := Zoom{Mode: ZoomFitPage}
	got := z.ScaleFactor(0, 400, 800, 600)
	if !almostEqual(got, 1.0) {
		t.Fatalf("ScaleFactor = %v, want 1.0 for degenerate page width", got)
	}
	got = z.ScaleFactor(200, 0, 800, 600)
	if !almostEqual(got, 1.0) {
		t.Fatalf("ScaleFactor = %v, want 1.0 for degenerate page height", got)
	}
}

func TestClampPercent(t *testing.T) {
	if got := ClampPercent(10); got != MinZoomPercent {
		t.Fatalf("ClampPercent(10) = %v, want %v", got, MinZoomPercent)
	}
	if got := ClampPercent(1000); got != MaxZoomPercent {
		t.Fatalf("ClampPercent(1000) = %v, want %v", got, MaxZoomPercent)
	}
	if got := ClampPercent(100); got != 100 {
		t.Fatalf("ClampPercent(100) = %v, want 100", got)
	}
}
