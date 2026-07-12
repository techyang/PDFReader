// internal/ui/thumbnails.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/pdfengine"
)

const thumbnailDPI = 24 // small enough to be fast; ~1/3 of a typical page's 72pt width in pixels

// buildThumbnails renders one small bitmap per page of doc and adds a
// clickable ImageView for each into parent (expected to be a ScrollView's
// content composite). onActivate is called with the 0-based page index
// when a thumbnail is clicked.
func buildThumbnails(parent walk.Container, doc *pdfengine.Document, onActivate func(page int)) error {
	for i := 0; i < doc.PageCount(); i++ {
		page := i
		img, err := doc.RenderPage(page, thumbnailDPI)
		if err != nil {
			return err
		}
		bmp, err := walk.NewBitmapFromImage(img)
		if err != nil {
			return err
		}

		iv, err := walk.NewImageView(parent)
		if err != nil {
			bmp.Dispose()
			return err
		}
		if err := iv.SetImage(bmp); err != nil {
			bmp.Dispose()
			return err
		}
		// Tie the bitmap's native GDI handle to the ImageView's lifetime: walk's
		// ImageView.SetImage only stores the reference (it never disposes a
		// previous image, and has no Dispose override of its own), and
		// WindowBase.Dispose only releases objects explicitly registered via
		// AddDisposable. Without this, every thumbnail leaks an HBITMAP for the
		// life of the process, not just the life of the tab.
		iv.AddDisposable(bmp)

		iv.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
			if button != walk.LeftButton {
				return
			}
			onActivate(page)
		})
	}
	return nil
}
