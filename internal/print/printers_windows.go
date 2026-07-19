// internal/print/printers_windows.go
package print

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
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
			names = append(names, windows.UTF16PtrToString(info.PPrinterName))
		}
	}

	defaultName, _ = defaultPrinterName()
	return names, defaultName, nil
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

	nameCount := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERNAMES, nil, nil)
	if int32(nameCount) <= 0 {
		return nil, fmt.Errorf("print: printer %q reports no paper names", printerName)
	}
	paperCount := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERS, nil, nil)
	if int32(paperCount) <= 0 {
		return nil, fmt.Errorf("print: printer %q reports no paper codes", printerName)
	}

	// nameBuf/codeBuf are each sized and filled to THEIR OWN capability's
	// count - DeviceCapabilities has no buffer-length parameter, so it
	// writes exactly as many entries as that capability's own null-query
	// reported, and giving it a smaller buffer than that would overflow it.
	// Only the combining loop below is bounded by the smaller of the two
	// counts, since a driver isn't guaranteed to report the same count for
	// both (documented as a convention, not enforced by the API) - reading
	// fewer entries than a buffer holds is safe, unlike writing more than
	// it holds.
	nameBuf := make([]uint16, int(nameCount)*paperNameEntryLen)
	if r := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERNAMES, &nameBuf[0], nil); int32(r) <= 0 {
		return nil, fmt.Errorf("print: DeviceCapabilities DC_PAPERNAMES failed for %q", printerName)
	}

	codeBuf := make([]int16, paperCount)
	if r := win.DeviceCapabilities(namePtr, nil, win.DC_PAPERS, (*uint16)(unsafe.Pointer(&codeBuf[0])), nil); int32(r) <= 0 {
		return nil, fmt.Errorf("print: DeviceCapabilities DC_PAPERS failed for %q", printerName)
	}

	count := nameCount
	if paperCount < count {
		count = paperCount
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
// user cancelled the dialog. base and the returned buffer are raw
// DEVMODE-sized byte buffers, not *win.DEVMODE - see gdi_windows.go's
// queryDevModeBuffer for why a fixed-size win.DEVMODE isn't safe to pass
// to DocumentProperties.
func QueryDevMode(ownerHWnd win.HWND, printerName string, base []byte) (buf []byte, ok bool) {
	namePtr, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return nil, false
	}

	if base != nil {
		buf = base
	} else {
		buf, err = queryDevModeBuffer(namePtr)
		if err != nil {
			return nil, false
		}
	}
	dm := (*win.DEVMODE)(unsafe.Pointer(&buf[0]))

	ret := win.DocumentProperties(ownerHWnd, 0, namePtr, dm, dm, win.DM_IN_BUFFER|win.DM_OUT_BUFFER|win.DM_IN_PROMPT)
	return buf, ret == win.IDOK
}
