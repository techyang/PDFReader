package pdfengine

import (
	"fmt"
	"image"

	"github.com/klippa-app/go-pdfium/requests"
)

// RenderPage renders the page at index (0-based) to an RGBA image at the
// given DPI.
func (d *Document) RenderPage(index int, dpi int) (*image.RGBA, error) {
	if index < 0 || index >= d.pages {
		return nil, fmt.Errorf("pdfengine: page index %d out of range [0,%d)", index, d.pages)
	}

	resp, err := d.instance.RenderPageInDPI(&requests.RenderPageInDPI{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: d.handle,
				Index:    index,
			},
		},
		DPI: dpi,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Cleanup()

	// Copy the pixels out because Cleanup() may release the underlying
	// WebAssembly memory backing resp.Result.Image.
	src := resp.Result.Image
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out, nil
}
