package pdfengine

import (
	"bytes"
	"testing"
)

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

	tests := []struct {
		name  string
		index int
	}{
		{"above upper bound", 5},
		{"negative index", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := doc.RenderPage(tt.index, 150); err == nil {
				t.Fatalf("RenderPage(%d, ...) expected error for out-of-range page", tt.index)
			}
		})
	}
}

// TestRenderPage_PixelDataIsCopied guards against RenderPage returning an
// *image.RGBA whose Pix slice aliases pdfium/WASM-owned memory instead of a
// real copy. If the copy in RenderPage were removed (e.g. "simplified" to
// `return src, nil`), a subsequent pdfium render call could reuse/overwrite
// the memory backing the first image, silently corrupting its pixels after
// the fact. This test renders a page, snapshots its pixel bytes, forces
// several more renders (to make memory reuse likely), and asserts the first
// image's bytes are still exactly what was returned.
func TestRenderPage_PixelDataIsCopied(t *testing.T) {
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

	img1, err := doc.RenderPage(0, 150)
	if err != nil {
		t.Fatalf("RenderPage(0, ...): %v", err)
	}

	// Sanity check: the rendered page must contain real raster content,
	// not a degenerate all-zero buffer, otherwise a corrupted/aliased
	// buffer could coincidentally still compare equal below.
	allZero := true
	for _, b := range img1.Pix {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("rendered page pixel data is all zero; cannot verify pixel copy")
	}

	snapshot := make([]byte, len(img1.Pix))
	copy(snapshot, img1.Pix)

	// Perform several more renders on the same pdfium instance. If
	// img1.Pix aliased WASM-owned memory freed/reused by Cleanup(), one
	// of these calls would overwrite img1's bytes in place.
	for i := 0; i < 5; i++ {
		if _, err := doc.RenderPage(1, 150); err != nil {
			t.Fatalf("RenderPage(1, ...) iteration %d: %v", i, err)
		}
		if _, err := doc.RenderPage(0, 150); err != nil {
			t.Fatalf("RenderPage(0, ...) iteration %d: %v", i, err)
		}
	}

	if !bytes.Equal(img1.Pix, snapshot) {
		t.Fatal("img1 pixel data changed after subsequent RenderPage calls; RenderPage is not copying pixel data out of pdfium/WASM-owned memory")
	}
}

func TestPageSize(t *testing.T) {
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

	w, h, err := doc.PageSize(0)
	if err != nil {
		t.Fatalf("PageSize: %v", err)
	}
	// A4 in points, from gofpdf: 595.28 x 841.89 (approximately).
	if w < 590 || w > 600 || h < 835 || h > 848 {
		t.Fatalf("PageSize = %vx%v, want ~595x842 (A4)", w, h)
	}
}
