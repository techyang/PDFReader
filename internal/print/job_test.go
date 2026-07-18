// internal/print/job_test.go
package print

import (
	"errors"
	"reflect"
	"testing"

	"pdfreader/internal/pdfengine"
)

type fakeJob struct {
	failOnPage int   // -1 = never fail
	closeErr   error // nil = Close never fails
	printed    []int
}

func (j *fakeJob) PrintPage(doc *pdfengine.Document, pageIndex int, settings Settings) error {
	if j.failOnPage >= 0 && pageIndex == j.failOnPage {
		return errors.New("fake print error")
	}
	j.printed = append(j.printed, pageIndex)
	return nil
}

func (j *fakeJob) Close() error { return j.closeErr }

type fakeBackend struct {
	failOpenFor map[string]bool
	failOnPage  int   // applied to every job this backend opens; -1 = never fail
	closeErr    error // applied to every job this backend opens; nil = Close never fails
	opened      []string
}

func (b *fakeBackend) Open(settings Settings, docName string) (BackendJob, error) {
	b.opened = append(b.opened, docName)
	if b.failOpenFor[docName] {
		return nil, errors.New("fake open error")
	}
	return &fakeJob{failOnPage: b.failOnPage, closeErr: b.closeErr}, nil
}

func newItem(path string, pageCount int, rangeSpec string) Item {
	return Item{Path: path, Doc: nil, PageCount: pageCount, RangeSpec: rangeSpec}
}

func TestRunJob_AllSucceed(t *testing.T) {
	b := &fakeBackend{failOnPage: -1}
	items := []Item{newItem("a.pdf", 2, ""), newItem("b.pdf", 1, "")}

	results := RunJob(b, items, Settings{}, func(Progress) bool { return false })

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("results[%d].Err = %v, want nil", i, r.Err)
		}
	}
	if !reflect.DeepEqual(b.opened, []string{"a.pdf", "b.pdf"}) {
		t.Fatalf("b.opened = %v, want [a.pdf b.pdf]", b.opened)
	}
}

func TestRunJob_SkipsFailedOpenContinues(t *testing.T) {
	b := &fakeBackend{failOnPage: -1, failOpenFor: map[string]bool{"a.pdf": true}}
	items := []Item{newItem("a.pdf", 2, ""), newItem("b.pdf", 1, "")}

	results := RunJob(b, items, Settings{}, func(Progress) bool { return false })

	if results[0].Err == nil {
		t.Fatalf("results[0].Err = nil, want an open error")
	}
	if results[1].Err != nil {
		t.Fatalf("results[1].Err = %v, want nil (must still run after a.pdf failed)", results[1].Err)
	}
}

func TestRunJob_PageFailureContinuesToNextFile(t *testing.T) {
	b := &fakeBackend{failOnPage: 0} // every job fails on its first page
	items := []Item{newItem("a.pdf", 2, ""), newItem("b.pdf", 1, "")}

	results := RunJob(b, items, Settings{}, func(Progress) bool { return false })

	if results[0].Err == nil {
		t.Fatalf("results[0].Err = nil, want a page error")
	}
	if results[1].Err == nil {
		t.Fatalf("results[1].Err = nil, want a page error (b.pdf's only page is page 0)")
	}
}

func TestRunJob_CancelStopsRemainingFiles(t *testing.T) {
	b := &fakeBackend{failOnPage: -1}
	items := []Item{newItem("a.pdf", 2, ""), newItem("b.pdf", 1, ""), newItem("c.pdf", 1, "")}

	calls := 0
	results := RunJob(b, items, Settings{}, func(Progress) bool {
		calls++
		return calls == 1 // cancel on the very first progress callback
	})

	if !errors.Is(results[0].Err, ErrCancelled) {
		t.Fatalf("results[0].Err = %v, want ErrCancelled", results[0].Err)
	}
	if !errors.Is(results[1].Err, ErrCancelled) || !errors.Is(results[2].Err, ErrCancelled) {
		t.Fatalf("results[1],[2].Err = %v, %v, want both ErrCancelled (never started)", results[1].Err, results[2].Err)
	}
	if len(b.opened) != 1 {
		t.Fatalf("b.opened = %v, want only a.pdf opened (b.pdf/c.pdf must never start)", b.opened)
	}
}

func TestRunJob_InvalidRangeSkipsFileWithoutOpening(t *testing.T) {
	b := &fakeBackend{failOnPage: -1}
	items := []Item{newItem("a.pdf", 3, "9-12"), newItem("b.pdf", 1, "")} // a.pdf's range is entirely out of bounds

	results := RunJob(b, items, Settings{}, func(Progress) bool { return false })

	if results[0].Err == nil {
		t.Fatalf("results[0].Err = nil, want a range-parse error")
	}
	if !reflect.DeepEqual(b.opened, []string{"b.pdf"}) {
		t.Fatalf("b.opened = %v, want only [b.pdf] (a.pdf must never reach Backend.Open)", b.opened)
	}
	if results[1].Err != nil {
		t.Fatalf("results[1].Err = %v, want nil", results[1].Err)
	}
}

func TestRunJob_EmptyItems(t *testing.T) {
	b := &fakeBackend{failOnPage: -1}
	results := RunJob(b, nil, Settings{}, func(Progress) bool { return false })
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestRunJob_ProgressReceivesCorrectFileAndPageCounts(t *testing.T) {
	b := &fakeBackend{failOnPage: -1}
	items := []Item{newItem("a.pdf", 3, "1,3")} // 2 pages selected out of 3

	var got []Progress
	RunJob(b, items, Settings{}, func(p Progress) bool {
		got = append(got, p)
		return false
	})

	want := []Progress{
		{FileIndex: 0, FileCount: 1, FileName: "a.pdf", Page: 1, PageCount: 2},
		{FileIndex: 0, FileCount: 1, FileName: "a.pdf", Page: 2, PageCount: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress calls = %+v, want %+v", got, want)
	}
}

func TestRunJob_CloseErrorSurfacesWhenNoPageError(t *testing.T) {
	closeErr := errors.New("fake close error")
	b := &fakeBackend{failOnPage: -1, closeErr: closeErr}
	items := []Item{newItem("a.pdf", 2, "")}

	results := RunJob(b, items, Settings{}, func(Progress) bool { return false })

	if !errors.Is(results[0].Err, closeErr) {
		t.Fatalf("results[0].Err = %v, want closeErr (%v)", results[0].Err, closeErr)
	}
}

func TestRunJob_PageErrorTakesPrecedenceOverCloseError(t *testing.T) {
	b := &fakeBackend{failOnPage: 0, closeErr: errors.New("fake close error")}
	items := []Item{newItem("a.pdf", 2, "")}

	results := RunJob(b, items, Settings{}, func(Progress) bool { return false })

	if results[0].Err == nil || errors.Is(results[0].Err, b.closeErr) {
		t.Fatalf("results[0].Err = %v, want the page error, not the close error", results[0].Err)
	}
}
