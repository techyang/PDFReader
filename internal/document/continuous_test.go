package document

import (
	"math"
	"testing"
)

func TestLayoutContinuous_SinglePageFitWidth(t *testing.T) {
	sizes := [][2]float64{{200, 400}} // 200x400pt page
	zoom := Zoom{Mode: ZoomFitWidth}
	layouts, total := LayoutContinuous(sizes, zoom, 800, 8)

	if len(layouts) != 1 {
		t.Fatalf("len(layouts) = %d, want 1", len(layouts))
	}
	l := layouts[0]
	if l.Top != 0 {
		t.Fatalf("layouts[0].Top = %v, want 0", l.Top)
	}
	wantScale := 800.0 / 200.0
	wantDPI := DPIForScale(wantScale)
	if l.DPI != wantDPI {
		t.Fatalf("layouts[0].DPI = %d, want %d", l.DPI, wantDPI)
	}
	wantHeight := 400.0 / 72.0 * float64(wantDPI)
	if math.Abs(l.Height-wantHeight) > 0.01 {
		t.Fatalf("layouts[0].Height = %v, want %v", l.Height, wantHeight)
	}
	if math.Abs(total-l.Height) > 0.01 {
		t.Fatalf("total = %v, want %v (no trailing gap for a single page)", total, l.Height)
	}
}

func TestLayoutContinuous_MultiPageCumulativeOffsets(t *testing.T) {
	sizes := [][2]float64{{200, 400}, {200, 300}, {200, 500}}
	zoom := Zoom{Mode: ZoomFitWidth}
	const gap = 10.0
	layouts, total := LayoutContinuous(sizes, zoom, 200, gap) // scale = 1.0, dpi = 72

	if len(layouts) != 3 {
		t.Fatalf("len(layouts) = %d, want 3", len(layouts))
	}
	if layouts[0].Top != 0 {
		t.Fatalf("layouts[0].Top = %v, want 0", layouts[0].Top)
	}
	wantTop1 := layouts[0].Height + gap
	if math.Abs(layouts[1].Top-wantTop1) > 0.01 {
		t.Fatalf("layouts[1].Top = %v, want %v", layouts[1].Top, wantTop1)
	}
	wantTop2 := wantTop1 + layouts[1].Height + gap
	if math.Abs(layouts[2].Top-wantTop2) > 0.01 {
		t.Fatalf("layouts[2].Top = %v, want %v", layouts[2].Top, wantTop2)
	}
	wantTotal := layouts[2].Top + layouts[2].Height
	if math.Abs(total-wantTotal) > 0.01 {
		t.Fatalf("total = %v, want %v", total, wantTotal)
	}
}

func TestLayoutContinuous_ZoomChangeScalesTotalHeight(t *testing.T) {
	sizes := [][2]float64{{200, 400}, {200, 400}}
	small := Zoom{Mode: ZoomPercent, Percent: 50}
	big := Zoom{Mode: ZoomPercent, Percent: 200}

	_, totalSmall := LayoutContinuous(sizes, small, 800, 8)
	_, totalBig := LayoutContinuous(sizes, big, 800, 8)

	if totalBig <= totalSmall*3 { // 200% vs 50% is 4x the linear scale
		t.Fatalf("totalBig = %v, totalSmall = %v, want totalBig much larger", totalBig, totalSmall)
	}
}

func TestVisiblePages_ViewportSpanningMultiplePages(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 108, Height: 100}, // 8px gap
		{Top: 216, Height: 100},
		{Top: 324, Height: 100},
	}

	start, end := VisiblePages(layouts, 150, 120) // viewport [150,270): touches pages 1,2

	if start != 1 || end != 3 {
		t.Fatalf("VisiblePages = [%d,%d), want [1,3)", start, end)
	}
}

func TestVisiblePages_GapOnlyViewportIsEmpty(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 108, Height: 100},
	}

	// viewport [100,108) sits entirely in the gap between the two pages.
	start, end := VisiblePages(layouts, 100, 8)

	if start != end {
		t.Fatalf("VisiblePages = [%d,%d), want an empty range in the gap", start, end)
	}
}

func TestVisiblePages_EmptyLayouts(t *testing.T) {
	start, end := VisiblePages(nil, 0, 100)
	if start != 0 || end != 0 {
		t.Fatalf("VisiblePages(nil) = [%d,%d), want [0,0)", start, end)
	}
}

func TestMostVisiblePage_PicksLargerOverlap(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 108, Height: 100},
	}

	// viewport [80,188): page 0 shows 20px (80..100), page 1 shows 80px (108..188).
	got := MostVisiblePage(layouts, 80, 108)

	if got != 1 {
		t.Fatalf("MostVisiblePage = %d, want 1", got)
	}
}

func TestMostVisiblePage_TieBreaksToEarlierPage(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 100, Height: 100},
	}

	// viewport [50,150): both pages show exactly 50px.
	got := MostVisiblePage(layouts, 50, 100)

	if got != 0 {
		t.Fatalf("MostVisiblePage = %d, want 0 (tie -> earlier page)", got)
	}
}

func TestMostVisiblePage_EmptyLayouts(t *testing.T) {
	if got := MostVisiblePage(nil, 0, 100); got != 0 {
		t.Fatalf("MostVisiblePage(nil) = %d, want 0", got)
	}
}
