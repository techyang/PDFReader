// internal/print/printers_windows.go
package print

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

// ListPrinterNames returns every local/connected printer's name, plus
// the name of the current Windows default printer (empty string if none
// is set or it can't be determined).
func ListPrinterNames() (names []string, defaultName string, err error) {
	const level = 4 // PRINTER_INFO_4: cheap (name + server + attributes only)
	flags := uint32(win.PRINTER_ENUM_LOCAL | win.PRINTER_ENUM_CONNECTIONS)

	var needed, returned uint32
	win.EnumPrinters(flags, nil, level, nil, 0, &needed, &returned)
	if needed == 0 {
		return nil, "", nil
	}

	buf := make([]byte, needed)
	if !win.EnumPrinters(flags, nil, level, &buf[0], needed, &needed, &returned) {
		return nil, "", fmt.Errorf("print: EnumPrinters failed")
	}

	infos := unsafe.Slice((*win.PRINTER_INFO_4)(unsafe.Pointer(&buf[0])), returned)
	names = make([]string, 0, returned)
	for _, info := range infos {
		if info.PPrinterName != nil {
			names = append(names, utf16PtrToString(info.PPrinterName))
		}
	}

	defaultName, _ = defaultPrinterName()
	return names, defaultName, nil
}

// utf16PtrToString converts a NUL-terminated UTF-16 string pointer (as
// returned in Win32 structs like PRINTER_INFO_4) to a Go string. The
// standard syscall package only offers UTF16ToString (slice-based), not
// a pointer variant, so this scans for the terminating NUL itself.
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), uintptr(n)*2)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

func defaultPrinterName() (string, error) {
	var size uint32
	win.GetDefaultPrinter(nil, &size)
	if size == 0 {
		return "", fmt.Errorf("print: no default printer set")
	}
	buf := make([]uint16, size)
	if !win.GetDefaultPrinter(&buf[0], &size) {
		return "", fmt.Errorf("print: GetDefaultPrinter failed")
	}
	return syscall.UTF16ToString(buf), nil
}

// PaperSize pairs a driver-reported paper name (what the dropdown shows)
// with its DMPAPER_* code (what actually goes into DEVMODE.DmPaperSize -
// see Settings.PaperCode).
type PaperSize struct {
	Name string
	Code int16
}

// paperNameEntryLen is the fixed width (in UTF-16 code units) of each
// entry in DeviceCapabilities' DC_PAPERNAMES output - a Win32 API
// contract (64 WCHARs per name, MSDN "DeviceCapabilities"), not
// something this binding can query.
const paperNameEntryLen = 64

// fallbackPaperSizes is used when ListPaperSizes fails (driver doesn't
// support the DC_PAPERNAMES query, or the printer name can't be
// resolved).
var fallbackPaperSizes = []PaperSize{
	{Name: "A4", Code: win.DMPAPER_A4},
	{Name: "Letter", Code: win.DMPAPER_LETTER},
	{Name: "Legal", Code: win.DMPAPER_LEGAL},
}

// ListPaperSizes returns the paper sizes printerName's driver reports
// supporting. Callers should fall back to fallbackPaperSizes on error.
func ListPaperSizes(printerName string) ([]PaperSize, error) {
	namePtr, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return nil, err
	}

	count := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERNAMES, nil, nil)
	if int32(count) <= 0 {
		return nil, fmt.Errorf("print: printer %q reports no paper names", printerName)
	}

	nameBuf := make([]uint16, int(count)*paperNameEntryLen)
	if r := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERNAMES, &nameBuf[0], nil); int32(r) <= 0 {
		return nil, fmt.Errorf("print: DeviceCapabilities DC_PAPERNAMES failed for %q", printerName)
	}

	codeBuf := make([]int16, count)
	if r := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERS, (*uint16)(unsafe.Pointer(&codeBuf[0])), nil); int32(r) <= 0 {
		return nil, fmt.Errorf("print: DeviceCapabilities DC_PAPERS failed for %q", printerName)
	}

	sizes := make([]PaperSize, 0, count)
	for i := 0; i < int(count); i++ {
		entry := nameBuf[i*paperNameEntryLen : (i+1)*paperNameEntryLen]
		name := syscall.UTF16ToString(entry)
		if name == "" {
			continue
		}
		sizes = append(sizes, PaperSize{Name: name, Code: codeBuf[i]})
	}
	return sizes, nil
}

// QueryDevMode opens the driver's native "属性" dialog (DocumentProperties
// with DM_IN_PROMPT) for printerName, seeded with base (or the driver's
// own defaults if base is nil), owned by ownerHWnd. ok is false if the
// user cancelled the dialog.
func QueryDevMode(ownerHWnd win.HWND, printerName string, base *win.DEVMODE) (dm win.DEVMODE, ok bool) {
	namePtr, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return win.DEVMODE{}, false
	}

	if base != nil {
		dm = *base
	} else {
		win.DocumentProperties(0, 0, namePtr, &dm, nil, win.DM_OUT_BUFFER)
	}

	ret := win.DocumentProperties(ownerHWnd, 0, namePtr, &dm, &dm, win.DM_IN_BUFFER|win.DM_OUT_BUFFER|win.DM_IN_PROMPT)
	return dm, ret == win.IDOK
}
