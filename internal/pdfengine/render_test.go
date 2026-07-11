package pdfengine

import "testing"

func TestRenderPage(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	img, err := doc.RenderPage(0, 150)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 1241 || bounds.Dy() != 1754 {
		t.Fatalf("rendered size = %dx%d, want 1241x1754", bounds.Dx(), bounds.Dy())
	}
}

func TestRenderPage_OutOfRange(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if _, err := doc.RenderPage(5, 150); err == nil {
		t.Fatal("RenderPage(5, ...) expected error for out-of-range page")
	}
}
