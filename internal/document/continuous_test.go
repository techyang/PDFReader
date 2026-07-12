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
