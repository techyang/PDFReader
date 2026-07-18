// internal/print/gdi_windows.go
package print

import (
	"fmt"
	"image"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"pdfreader/internal/pdfengine"
)

// renderDPI is the fixed resolution every page is rasterized at before
// being scaled onto the physical page: high enough for text/chart PDFs,
// low enough to keep memory/time bounded regardless of the printer's
// own (possibly much higher) native DPI.
const renderDPI = 300

// StretchDIBits isn't bound anywhere in github.com/lxn/win (there is no
// match for it anywhere in that module), so it's declared here directly,
// following the exact same LazyDLL/LazyProc + raw syscall.SyscallN
// pattern lxn/win itself uses throughout (e.g. winspool.go's EnumPrinters).
var (
	libgdi32          = windows.NewLazySystemDLL("gdi32.dll")
	procStretchDIBits = libgdi32.NewProc("StretchDIBits")
)

// stretchDIBits wraps the Win32 StretchDIBits API (13 parameters -
// syscall.Syscall15 is the smallest of Go's fixed-arity syscall helpers
// that covers it, with the last 2 slots padded with 0).
func stretchDIBits(hdc win.HDC, xDest, yDest, destW, destH, xSrc, ySrc, srcW, srcH int32, bits unsafe.Pointer, bmi *win.BITMAPINFO, usage, rop uint32) int32 {
	ret, _, _ := syscall.Syscall15(procStretchDIBits.Addr(), 13,
		uintptr(hdc),
		uintptr(xDest),
		uintptr(yDest),
		uintptr(destW),
		uintptr(destH),
		uintptr(xSrc),
		uintptr(ySrc),
		uintptr(srcW),
		uintptr(srcH),
		uintptr(bits),
		uintptr(unsafe.Pointer(bmi)),
		uintptr(usage),
		uintptr(rop),
		0, 0)
	return int32(ret)
}

// gdiBackend is the real Backend, implemented directly on Windows GDI
// print APIs.
type gdiBackend struct{}

// NewGDIBackend returns the Backend RunJob should use for real printing
// (as opposed to the fake used in job_test.go).
func NewGDIBackend() Backend { return gdiBackend{} }

func (gdiBackend) Open(settings Settings, docName string) (BackendJob, error) {
	driverPtr, err := syscall.UTF16PtrFromString("WINSPOOL")
	if err != nil {
		return nil, err
	}
	namePtr, err := syscall.UTF16PtrFromString(settings.PrinterName)
	if err != nil {
		return nil, err
	}

	var dm win.DEVMODE
	if settings.BaseDevMode != nil {
		dm = *settings.BaseDevMode
	} else {
		// DM_OUT_BUFFER reads the printer's current driver-default DEVMODE
		// into dm; buildDevMode below only needs to override the handful
		// of fields our own dialog exposes.
		win.DocumentProperties(0, 0, namePtr, &dm, nil, win.DM_OUT_BUFFER)
	}
	buildDevMode(&dm, settings)

	hdc := win.CreateDC(driverPtr, namePtr, nil, &dm)
	if hdc == 0 {
		return nil, fmt.Errorf("print: CreateDC failed for printer %q", settings.PrinterName)
	}

	// filepath.Base: docName is the file's full path (see Item.Path);
	// the print spooler's job-name display should show just the
	// filename, matching what a user would expect to see in the print
	// queue window, not their whole local path.
	docNamePtr, err := syscall.UTF16PtrFromString(filepath.Base(docName))
	if err != nil {
		win.DeleteDC(hdc)
		return nil, err
	}
	di := win.DOCINFO{
		CbSize:      int32(unsafe.Sizeof(win.DOCINFO{})),
		LpszDocName: docNamePtr,
	}
	if win.StartDoc(hdc, &di) <= 0 {
		win.DeleteDC(hdc)
		return nil, fmt.Errorf("print: StartDoc failed for %q", docName)
	}

	return &gdiJob{hdc: hdc}, nil
}

// buildDevMode applies settings' fields on top of dm (which already
// holds either the printer's driver defaults or the "属性" dialog's
// last result - see Settings.BaseDevMode). This always wins over
// whatever "属性" set for these specific four fields, by design - our
// own checkboxes/dropdowns are meant to be the last word on
// copies/duplex/orientation/paper for a print triggered from this dialog.
func buildDevMode(dm *win.DEVMODE, s Settings) {
	copies := int16(s.Copies)
	if copies < 1 {
		copies = 1
	}
	dm.DmFields |= win.DM_COPIES
	dm.DmCopies = copies

	dm.DmFields |= win.DM_ORIENTATION
	if s.Orientation == OrientLandscape {
		dm.DmOrientation = win.DMORIENT_LANDSCAPE
	} else {
		dm.DmOrientation = win.DMORIENT_PORTRAIT
	}

	dm.DmFields |= win.DM_DUPLEX
	if s.Duplex {
		dm.DmDuplex = win.DMDUP_VERTICAL
	} else {
		dm.DmDuplex = win.DMDUP_SIMPLEX
	}

	// dmColor is only a hint some drivers honor - toDIBBits's software
	// grayscale conversion below is what actually guarantees grayscale
	// output regardless of driver support.
	if s.Grayscale {
		dm.DmFields |= win.DM_COLOR
		dm.DmColor = win.DMCOLOR_MONOCHROME
	}

	if s.PaperCode != 0 {
		dm.DmFields |= win.DM_PAPERSIZE
		dm.DmPaperSize = s.PaperCode
	}
}

type gdiJob struct {
	hdc win.HDC
}

func (j *gdiJob) PrintPage(doc *pdfengine.Document, pageIndex int, settings Settings) error {
	if win.StartPage(j.hdc) <= 0 {
		return fmt.Errorf("print: StartPage failed")
	}

	img, err := doc.RenderPage(pageIndex, renderDPI)
	if err != nil {
		win.EndPage(j.hdc)
		return err
	}
	widthPt, heightPt, err := doc.PageSize(pageIndex)
	if err != nil {
		win.EndPage(j.hdc)
		return err
	}

	canvasW := win.GetDeviceCaps(j.hdc, win.HORZRES)
	canvasH := win.GetDeviceCaps(j.hdc, win.VERTRES)
	dpiX := win.GetDeviceCaps(j.hdc, win.LOGPIXELSX)
	dpiY := win.GetDeviceCaps(j.hdc, win.LOGPIXELSY)

	imgW, imgH := img.Rect.Dx(), img.Rect.Dy()
	x, y, w, h := destRect(imgW, imgH, widthPt, heightPt, dpiX, dpiY, canvasW, canvasH, settings.ScaleMode, settings.ScalePercent)

	bits, bmiHeader := toDIBBits(img, settings.Grayscale)
	bmi := win.BITMAPINFO{BmiHeader: bmiHeader}
	if stretchDIBits(j.hdc, x, y, w, h, 0, 0, int32(imgW), int32(imgH), unsafe.Pointer(&bits[0]), &bmi, win.DIB_RGB_COLORS, win.SRCCOPY) <= 0 {
		win.EndPage(j.hdc)
		return fmt.Errorf("print: StretchDIBits failed")
	}

	if win.EndPage(j.hdc) <= 0 {
		return fmt.Errorf("print: EndPage failed")
	}
	return nil
}

func (j *gdiJob) Close() error {
	endDocResult := win.EndDoc(j.hdc)
	win.DeleteDC(j.hdc)
	if endDocResult <= 0 {
		return fmt.Errorf("print: EndDoc failed")
	}
	return nil
}

// toDIBBits converts img (as returned by pdfengine.Document.RenderPage -
// row 0 is the top row) into a top-down 32bpp BGRA buffer plus a matching
// BITMAPINFOHEADER, ready for stretchDIBits. The actual pixel conversion
// (channel swap, grayscale) lives in bgraFromRGBA (dib.go) since it has
// no Windows dependency and can be unit-tested there; this function just
// wraps that buffer in the Windows-specific bitmap header.
func toDIBBits(img *image.RGBA, grayscale bool) ([]byte, win.BITMAPINFOHEADER) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := bgraFromRGBA(img, grayscale)
	return out, win.BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
		BiWidth:       int32(w),
		BiHeight:      -int32(h), // negative = top-down, matches image.RGBA's row order (no flip needed)
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: win.BI_RGB,
		BiSizeImage:   uint32(len(out)),
	}
}
