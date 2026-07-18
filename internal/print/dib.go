// internal/print/dib.go
package print

import "image"

// bgraFromRGBA converts img (as returned by pdfengine.Document.RenderPage
// - row 0 is the top row) into a top-down 32bpp BGRA pixel buffer, ready
// to be wrapped in a platform-specific bitmap header (see gdi_windows.go's
// toDIBBits). Windows DIBs store pixels as B,G,R,A/X - the reverse
// channel order from Go's image.RGBA (R,G,B,A) - so this always swaps
// channels; grayscale is applied here too (channel-order-agnostic, so it
// doesn't matter whether it happens before or after the swap).
func bgraFromRGBA(img *image.RGBA, grayscale bool) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		srcRow := img.Pix[y*img.Stride : y*img.Stride+w*4]
		dstRow := out[y*w*4 : y*w*4+w*4]
		for x := 0; x < w; x++ {
			r, g, b, a := srcRow[x*4], srcRow[x*4+1], srcRow[x*4+2], srcRow[x*4+3]
			if grayscale {
				gray := uint8((uint32(r)*299 + uint32(g)*587 + uint32(b)*114) / 1000)
				r, g, b = gray, gray, gray
			}
			dstRow[x*4] = b
			dstRow[x*4+1] = g
			dstRow[x*4+2] = r
			dstRow[x*4+3] = a
		}
	}
	return out
}
