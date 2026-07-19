// internal/print/job.go
package print

import (
	"errors"

	"pdfreader/internal/pdfengine"
)

// ErrCancelled marks a Result for a file that was skipped (or interrupted
// mid-way) because the user cancelled a batch print job - see RunJob.
var ErrCancelled = errors.New("print: cancelled")

// Settings is shared across every file in one batch print job - the only
// per-file setting is each Item's RangeSpec below.
type Settings struct {
	PrinterName  string
	Copies       int
	Grayscale    bool
	Duplex       bool
	PaperCode    int16 // win.DEVMODE.DmPaperSize; 0 = don't override the printer's default
	Orientation  Orientation
	ScaleMode    ScaleMode
	ScalePercent int // only meaningful when ScaleMode == ScalePercent

	// BaseDevMode, if non-nil, is used as the starting DEVMODE buffer
	// instead of re-querying the printer's driver defaults - this is how
	// the "属性" button's result (DocumentProperties with DM_IN_PROMPT)
	// flows into the actual print job. It's a raw byte buffer rather than
	// *win.DEVMODE because a driver's real DEVMODE can be larger than
	// win.DEVMODE's fixed struct (drivers may append private "extra" data
	// - see win.DEVMODE.DmDriverExtra); gdi_windows.go's queryDevModeBuffer
	// allocates it at the driver-reported size and casts a *win.DEVMODE
	// onto the front of it. The fields above are still applied on top of
	// it (see gdi_windows.go's buildDevMode), so e.g. a duplex setting
	// changed via "属性" is overridden by this dialog's own "双面打印"
	// checkbox.
	BaseDevMode []byte
}

// Item is one file in a batch print job.
type Item struct {
	Path      string
	Doc       *pdfengine.Document
	PageCount int
	RangeSpec string // "" means all pages - see ParseRange
}

// Result is RunJob's outcome for one Item. Err is nil on success,
// ErrCancelled if the file was never started (or interrupted) because
// the batch was cancelled, or any other error (open/page-render/print
// failure, or an invalid RangeSpec) otherwise.
type Result struct {
	Item Item
	Err  error
}

// Progress is reported to RunJob's callback right before each page is
// sent to the printer.
type Progress struct {
	FileIndex int // 0-based index into the Items slice passed to RunJob
	FileCount int
	FileName  string
	Page      int // 1-based page number within this file's *selected* range, in print order
	PageCount int // number of pages being printed for this file (len of the parsed range)
}

// Backend abstracts the actual printer calls RunJob drives, so the
// orchestration logic below (skip-on-failure, cancellation, progress
// reporting) can be unit-tested without a real Windows print subsystem.
// The real implementation (gdiBackend, a later task) is constructed via
// NewGDIBackend.
type Backend interface {
	// Open starts a new print job (DEVMODE setup, CreateDC, StartDoc) for
	// one file. docName is used as the spooler job's display name.
	Open(settings Settings, docName string) (BackendJob, error)
}

// BackendJob is one open print job (one file), spanning StartDoc..EndDoc.
type BackendJob interface {
	// PrintPage renders and prints one page (StartPage, blit, EndPage).
	PrintPage(doc *pdfengine.Document, pageIndex int, settings Settings) error
	// Close finishes the job (EndDoc, DeleteDC). RunJob calls this exactly
	// once per successful Open, even if PrintPage returned an error.
	Close() error
}

// RunJob prints every item in items using settings, calling progress
// before each page (progress returning true requests cancellation).
//
// A single item failing - Backend.Open erroring, a page failing to
// render/print, or its RangeSpec failing ParseRange - is recorded in the
// returned []Result and does NOT stop the rest of the batch; only an
// explicit cancellation does. Once cancelled, every remaining item
// (including the one that was mid-way through when cancel was requested)
// gets Result.Err = ErrCancelled, and no further Backend.Open calls happen.
func RunJob(b Backend, items []Item, settings Settings, progress func(Progress) bool) []Result {
	results := make([]Result, 0, len(items))
	cancelled := false

	for i, item := range items {
		if cancelled {
			results = append(results, Result{Item: item, Err: ErrCancelled})
			continue
		}

		pages, err := ParseRange(item.RangeSpec, item.PageCount)
		if err != nil {
			results = append(results, Result{Item: item, Err: err})
			continue
		}

		job, err := b.Open(settings, item.Path)
		if err != nil {
			results = append(results, Result{Item: item, Err: err})
			continue
		}

		var pageErr error
		for p, pageIndex := range pages {
			if progress(Progress{FileIndex: i, FileCount: len(items), FileName: item.Path, Page: p + 1, PageCount: len(pages)}) {
				cancelled = true
				pageErr = ErrCancelled
				break
			}
			if err := job.PrintPage(item.Doc, pageIndex, settings); err != nil {
				pageErr = err
				break
			}
		}

		closeErr := job.Close()
		if pageErr == nil {
			pageErr = closeErr
		}

		results = append(results, Result{Item: item, Err: pageErr})
	}

	return results
}
