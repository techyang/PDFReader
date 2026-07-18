// internal/print/dib_test.go
package print

import (
	"image"
	"image/color"
	"testing"
)

// newTestRGBA builds a 2x2 image.RGBA with four distinct, hand-picked
// pixels so channel-swap and grayscale-weighting bugs are easy to spot
// from the expected values computed alongside them. Using color.RGBA
// (not color.NRGBA or a custom type) means img.Set stores these bytes
// verbatim in img.Pix, with no premultiplication surprises.
func newTestRGBA() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 40})
	img.Set(1, 0, color.RGBA{R: 100, G: 150, B: 200, A: 255})
	img.Set(0, 1, color.RGBA{R: 0, G: 0, B: 0, A: 0})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 128})
	return img
}

func TestBgraFromRGBA_ChannelSwap(t *testing.T) {
	img := newTestRGBA()
	out := bgraFromRGBA(img, false)

	want := make([]byte, len(img.Pix))
	copy(want, img.Pix)
	for i := 0; i < len(want); i += 4 {
		want[i], want[i+2] = want[i+2], want[i] // R and B swapped, G/A unchanged
	}

	if len(out) != len(want) {
		t.Fatalf("output length = %d, want %d", len(out), len(want))
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("byte %d = %d, want %d", i, out[i], want[i])
		}
	}
}

func TestBgraFromRGBA_Grayscale(t *testing.T) {
	img := newTestRGBA()
	out := bgraFromRGBA(img, true)

	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			srcOff := y*img.Stride + x*4
			r, g, b := img.Pix[srcOff], img.Pix[srcOff+1], img.Pix[srcOff+2]
			wantGray := uint8((uint32(r)*299 + uint32(g)*587 + uint32(b)*114) / 1000)

			dstOff := (y*2 + x) * 4
			gotB, gotG, gotR := out[dstOff], out[dstOff+1], out[dstOff+2]
			if gotB != wantGray || gotG != wantGray || gotR != wantGray {
				t.Errorf("pixel (%d,%d): got B=%d G=%d R=%d, want all == %d", x, y, gotB, gotG, gotR, wantGray)
			}
			if gotB != gotG || gotG != gotR {
				t.Errorf("pixel (%d,%d): B/G/R not equal after grayscale: B=%d G=%d R=%d", x, y, gotB, gotG, gotR)
			}
		}
	}
}

func TestBgraFromRGBA_OutputLength(t *testing.T) {
	cases := []struct{ w, h int }{
		{1, 1},
		{2, 1},
		{1, 2},
		{3, 5},
		{300, 300},
	}
	for _, c := range cases {
		img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
		for _, gray := range []bool{false, true} {
			out := bgraFromRGBA(img, gray)
			want := c.w * c.h * 4
			if len(out) != want {
				t.Errorf("w=%d h=%d gray=%v: output length = %d, want %d", c.w, c.h, gray, len(out), want)
			}
		}
	}
}
