# 批量打印 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 PDF 阅读器加一个统一的「打印」对话框（Ctrl+P），左侧文件列表 + 右侧共享设置面板，列表只有一个文件时就是单文件打印，多个文件时就是批量打印。

**Architecture:** 新增 `internal/print` 包做打印机枚举、DEVMODE 构造、页码范围解析、GDI 打印管线（`Backend`/`BackendJob` 接口 + 可测的 `RunJob` 编排逻辑，真正的 GDI 调用是一份 build-tag 为 windows 的实现）；`internal/ui/printdialog.go` 是对话框 UI，调用 `internal/print`；`internal/ui/app.go` 加菜单入口和进度弹窗。

**Tech Stack:** Go 1.25+，`github.com/lxn/walk`（GUI）、`github.com/lxn/win`（原始 GDI/打印 syscall 绑定）、`github.com/klippa-app/go-pdfium`（PDF 渲染，已有）。

**设计文档：** `docs/superpowers/specs/2026-07-18-batch-print-design.md`（先读一遍，本计划的每个任务都对应设计文档里的某一节）。

**关于跨平台的一个偏离说明：** 设计文档暗示 `internal/print` 应该像 `internal/document` 一样尽量平台无关。实际实现时发现「属性」按钮返回的 `*win.DEVMODE` 需要一路传到 GDI 后端，最干净的办法是让 `Settings`（定义在不带 build tag 的 `job.go`）直接持有一个 `*win.DEVMODE` 字段。这个仓库本来就只为 Windows 构建（`internal/ui` 全程依赖 `lxn/walk`，后者所有源文件都是 `+build windows`），所以 `internal/print` 引入 `github.com/lxn/win` 不会破坏任何实际可移植性——只有 `pagerange.go`/`layout.go` 这两个纯计算文件保持不需要 `lxn/win`，不是因为要跨平台，而是因为它们确实用不上。

---

## 文件结构总览

```
internal/print/
  pagerange.go        # ParseRange 纯函数（TDD）
  pagerange_test.go
  layout.go           # ScaleMode/Orientation/destRect 纯几何计算（TDD）
  layout_test.go
  job.go              # Settings/Item/Result/Progress/Backend/BackendJob/RunJob（TDD，假 Backend）
  job_test.go
  gdi_windows.go       # 真正的 GDI 实现（CreateDC/StartDoc/StretchDIBits...），文件名后缀 _windows.go 让 go 工具链自动只在 windows 下编译
  printers_windows.go  # EnumPrinters/GetDefaultPrinter/DeviceCapabilities 纸张查询

internal/config/
  config.go           # 加 LastPrinter 等字段（改动）
  config_test.go       # 加对应 round-trip 测试（改动）

internal/ui/
  printdialog.go       # 打印对话框：文件列表 + 设置面板 + 进度弹窗
  app.go               # 加「打印...」菜单项、Ctrl+P（改动）

README.md               # 手动测试清单追加打印相关条目（改动）
```

---

### Task 1: `internal/print.ParseRange`

**Files:**
- Create: `internal/print/pagerange.go`
- Test: `internal/print/pagerange_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/print/pagerange_test.go
package print

import (
	"reflect"
	"testing"
)

func TestParseRange_EmptyMeansAllPages(t *testing.T) {
	got, err := ParseRange("", 3)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"\", 3) = %v, want %v", got, want)
	}
}

func TestParseRange_CommaAndDash(t *testing.T) {
	got, err := ParseRange("1,3-5", 6)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 2, 3, 4} // 1-based "1,3-5" -> 0-based {0,2,3,4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"1,3-5\", 6) = %v, want %v", got, want)
	}
}

func TestParseRange_DedupesAndSorts(t *testing.T) {
	got, err := ParseRange("3,1,2-3,1", 5)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"3,1,2-3,1\", 5) = %v, want %v", got, want)
	}
}

func TestParseRange_OutOfRangePagesSilentlyDropped(t *testing.T) {
	got, err := ParseRange("2,9999", 5)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"2,9999\", 5) = %v, want %v", got, want)
	}
}

func TestParseRange_AllOutOfRangeIsError(t *testing.T) {
	_, err := ParseRange("9-12", 5)
	if err == nil {
		t.Fatalf("ParseRange(\"9-12\", 5) = nil error, want error (no valid pages)")
	}
}

func TestParseRange_InvalidTokenIsError(t *testing.T) {
	_, err := ParseRange("abc", 5)
	if err == nil {
		t.Fatalf("ParseRange(\"abc\", 5) = nil error, want error")
	}
}

func TestParseRange_ReversedRangeIsError(t *testing.T) {
	_, err := ParseRange("5-1", 5)
	if err == nil {
		t.Fatalf("ParseRange(\"5-1\", 5) = nil error, want error (start > end)")
	}
}

func TestParseRange_EmptyTokensSkipped(t *testing.T) {
	got, err := ParseRange("1,,3", 5)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"1,,3\", 5) = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/print/... -run TestParseRange -v`
Expected: FAIL，报 `undefined: ParseRange`（`pagerange.go` 还不存在）

- [ ] **Step 3: 写实现**

```go
// internal/print/pagerange.go
package print

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ParseRange parses a 1-based page-range spec like "1,8,9-12" (the
// syntax shown to the user in the print dialog's "选择页" box) into a
// sorted, de-duplicated list of 0-based page indices within
// [0, pageCount). An empty spec means "all pages".
//
// Page numbers outside [1, pageCount] are silently dropped rather than
// treated as an error - different files in a batch have different page
// counts, so referencing a page that doesn't exist in THIS file is
// expected, not a mistake (see
// docs/superpowers/specs/2026-07-18-batch-print-design.md section 7).
// But if parsing yields zero pages at all - either every number was out
// of range, or a malformed token (not a number, not a valid "N-M" range,
// or a range with start > end) - ParseRange returns an error, so the
// caller can record the whole file as failed with reason "页码范围无效"
// rather than silently print nothing.
func ParseRange(spec string, pageCount int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		if pageCount <= 0 {
			return nil, fmt.Errorf("print: page count must be positive, got %d", pageCount)
		}
		pages := make([]int, pageCount)
		for i := range pages {
			pages[i] = i
		}
		return pages, nil
	}

	seen := make(map[int]bool)
	var pages []int
	add := func(n int) {
		if n < 1 || n > pageCount {
			return
		}
		idx := n - 1
		if !seen[idx] {
			seen[idx] = true
			pages = append(pages, idx)
		}
	}

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if dash := strings.IndexByte(part, '-'); dash > 0 {
			start, err := strconv.Atoi(strings.TrimSpace(part[:dash]))
			if err != nil {
				return nil, fmt.Errorf("print: invalid range %q", part)
			}
			end, err := strconv.Atoi(strings.TrimSpace(part[dash+1:]))
			if err != nil {
				return nil, fmt.Errorf("print: invalid range %q", part)
			}
			if start > end {
				return nil, fmt.Errorf("print: invalid range %q (start > end)", part)
			}
			for n := start; n <= end; n++ {
				add(n)
			}
			continue
		}

		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("print: invalid page number %q", part)
		}
		add(n)
	}

	sort.Ints(pages)

	if len(pages) == 0 {
		return nil, fmt.Errorf("print: no valid pages in range %q for a %d-page document", spec, pageCount)
	}
	return pages, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/print/... -run TestParseRange -v`
Expected: PASS（全部 8 个子测试）

- [ ] **Step 5: 提交**

```bash
git add internal/print/pagerange.go internal/print/pagerange_test.go
git commit -m "$(cat <<'EOF'
feat(print): add page-range spec parsing

ParseRange turns a "1,8,9-12" style user input into sorted, de-duped
0-based page indices, silently dropping out-of-range page numbers (each
file in a batch has a different page count) but erroring when a spec
resolves to zero valid pages or contains a malformed token.
EOF
)"
```

---

### Task 2: `internal/print` 缩放/定位几何计算

**Files:**
- Create: `internal/print/layout.go`
- Test: `internal/print/layout_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/print/layout_test.go
package print

import "testing"

func TestDestRect_FitPage_PreservesAspectAndCenters(t *testing.T) {
	// 1000x2000 px image (portrait, taller than wide) into a 3000x3000 square canvas.
	x, y, w, h := destRect(1000, 2000, 0, 0, 300, 300, 3000, 3000, ScaleFitPage, 0)
	if w != 1500 || h != 3000 {
		t.Fatalf("w,h = %d,%d, want 1500,3000 (scaled by canvasH/imgH=1.5, capped by height)", w, h)
	}
	if x != 750 || y != 0 {
		t.Fatalf("x,y = %d,%d, want 750,0 (centered horizontally, flush vertically)", x, y)
	}
}

func TestDestRect_ActualSize_UsesPrinterDPI(t *testing.T) {
	// A 72pt x 144pt (1in x 2in) page at 300 DPI printer resolution should be
	// exactly 300x600 device px, regardless of the source image's own pixel size.
	x, y, w, h := destRect(300, 600, 72, 144, 300, 300, 3000, 3000, ScaleActualSize, 0)
	if w != 300 || h != 600 {
		t.Fatalf("w,h = %d,%d, want 300,600", w, h)
	}
	wantX, wantY := int32((3000-300)/2), int32((3000-600)/2)
	if x != wantX || y != wantY {
		t.Fatalf("x,y = %d,%d, want %d,%d (centered)", x, y, wantX, wantY)
	}
}

func TestDestRect_Percent_ScalesActualSize(t *testing.T) {
	// Same page as above, at 50% -> half the actual-size dimensions.
	_, _, w, h := destRect(300, 600, 72, 144, 300, 300, 3000, 3000, ScalePercent, 50)
	if w != 150 || h != 300 {
		t.Fatalf("w,h = %d,%d, want 150,300", w, h)
	}
}

func TestDestRect_NeverNegativeOffset(t *testing.T) {
	// Image bigger than the canvas (e.g. actual-size overflowing a small
	// canvas) must clamp x/y to 0, not go negative.
	x, y, _, _ := destRect(300, 600, 500, 1000, 300, 300, 100, 100, ScaleActualSize, 0)
	if x != 0 || y != 0 {
		t.Fatalf("x,y = %d,%d, want 0,0 (clamped)", x, y)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/print/... -run TestDestRect -v`
Expected: FAIL，报 `undefined: destRect`/`undefined: ScaleFitPage` 等

- [ ] **Step 3: 写实现**

```go
// internal/print/layout.go
package print

// ScaleMode controls how a rendered page bitmap is scaled onto the
// physical printable area when it doesn't exactly match the paper size -
// see docs/superpowers/specs/2026-07-18-batch-print-design.md section 2
// ("页面大小调整").
type ScaleMode int

const (
	ScaleFitPage    ScaleMode = iota // scale to fill the page, aspect preserved
	ScaleActualSize                  // 1 PDF point = 1/72 inch on paper, no user scaling
	ScalePercent                    // like ScaleActualSize, then multiplied by Settings.ScalePercent/100
)

// Orientation mirrors DEVMODE's DMORIENT_* values, defined here (instead
// of importing lxn/win into this platform-agnostic file) purely so
// job.go/layout.go don't need a Windows-only dependency for something
// this simple.
type Orientation int

const (
	OrientPortrait Orientation = iota
	OrientLandscape
)

// destRect computes where on the printable canvas (device pixels, origin
// at the printable area's top-left, size canvasW x canvasH) a rendered
// page image of size imgW x imgH pixels should be drawn.
//
// widthPt/heightPt are the PDF page's real size in points (1/72 inch,
// from pdfengine.Document.PageSize) and dpiX/dpiY are the PRINTER's own
// device DPI (GetDeviceCaps LOGPIXELSX/Y) - both are only used by
// ScaleActualSize/ScalePercent to compute the page's true physical size
// on paper, independent of whatever DPI the source image was rendered
// at (see gdi_windows.go's renderDPI constant - always 300, regardless
// of the printer's real resolution). ScaleFitPage ignores widthPt/
// heightPt/dpiX/dpiY entirely: the image already has the right aspect
// ratio, so it's fit to canvasW x canvasH directly.
//
// The result is always centered within the canvas and clamped so x/y
// are never negative, even if the requested size exceeds the canvas
// (e.g. actual-size on a page bigger than the paper) - mirrors the same
// clamp-at-0 centering already used for on-screen single-page painting
// in pageview.go's paintTab.
func destRect(imgW, imgH int, widthPt, heightPt float64, dpiX, dpiY, canvasW, canvasH int32, mode ScaleMode, percent int) (x, y, w, h int32) {
	switch mode {
	case ScaleActualSize, ScalePercent:
		factor := 1.0
		if mode == ScalePercent {
			factor = float64(percent) / 100.0
		}
		w = int32(widthPt / 72 * float64(dpiX) * factor)
		h = int32(heightPt / 72 * float64(dpiY) * factor)
	default: // ScaleFitPage
		scale := float64(canvasW) / float64(imgW)
		if alt := float64(canvasH) / float64(imgH); alt < scale {
			scale = alt
		}
		w = int32(float64(imgW) * scale)
		h = int32(float64(imgH) * scale)
	}

	x = (canvasW - w) / 2
	y = (canvasH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y, w, h
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/print/... -run TestDestRect -v`
Expected: PASS（全部 4 个子测试）

- [ ] **Step 5: 提交**

```bash
git add internal/print/layout.go internal/print/layout_test.go
git commit -m "$(cat <<'EOF'
feat(print): add page-to-paper scaling/centering geometry

destRect computes the device-pixel rect a rendered page image should be
drawn into for each of the three scale modes (fit-page/actual-size/
percent), always centered and clamped to non-negative offsets - kept as
pure, platform-agnostic logic so it's unit-testable without a real
printer.
EOF
)"
```

---

### Task 3: `internal/print` 任务编排（`RunJob`）

**Files:**
- Create: `internal/print/job.go`
- Test: `internal/print/job_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/print/job_test.go
package print

import (
	"errors"
	"reflect"
	"testing"

	"pdfreader/internal/pdfengine"
)

type fakeJob struct {
	failOnPage int // -1 = never fail
	printed    []int
}

func (j *fakeJob) PrintPage(doc *pdfengine.Document, pageIndex int, settings Settings) error {
	if j.failOnPage >= 0 && pageIndex == j.failOnPage {
		return errors.New("fake print error")
	}
	j.printed = append(j.printed, pageIndex)
	return nil
}

func (j *fakeJob) Close() error { return nil }

type fakeBackend struct {
	failOpenFor map[string]bool
	failOnPage  int // applied to every job this backend opens; -1 = never fail
	opened      []string
}

func (b *fakeBackend) Open(settings Settings, docName string) (BackendJob, error) {
	b.opened = append(b.opened, docName)
	if b.failOpenFor[docName] {
		return nil, errors.New("fake open error")
	}
	return &fakeJob{failOnPage: b.failOnPage}, nil
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/print/... -run TestRunJob -v`
Expected: FAIL，报 `undefined: Item`/`undefined: Settings`/`undefined: RunJob` 等（`job.go` 还不存在）

- [ ] **Step 3: 写实现**

```go
// internal/print/job.go
package print

import (
	"errors"

	"github.com/lxn/win"

	"pdfreader/internal/pdfengine"
)

// ErrCancelled marks a Result for a file that was skipped (or interrupted
// mid-way) because the user cancelled a batch print job - see RunJob.
var ErrCancelled = errors.New("print: cancelled")

// Settings is shared across every file in one batch print job - see
// docs/superpowers/specs/2026-07-18-batch-print-design.md section 2, the
// only per-file setting is each Item's RangeSpec below.
type Settings struct {
	PrinterName  string
	Copies       int
	Grayscale    bool
	Duplex       bool
	PaperCode    int16 // win.DEVMODE.DmPaperSize; 0 = don't override the printer's default
	Orientation  Orientation
	ScaleMode    ScaleMode
	ScalePercent int // only meaningful when ScaleMode == ScalePercent

	// BaseDevMode, if non-nil, is used as the starting DEVMODE instead of
	// re-querying the printer's driver defaults - this is how the "属性"
	// button's result (DocumentProperties with DM_IN_PROMPT) flows into
	// the actual print job. The fields above are still applied on top of
	// it (see gdi_windows.go's buildDevMode), so e.g. a duplex setting
	// changed via "属性" is overridden by this dialog's own "双面打印"
	// checkbox - see design doc discussion in printdialog.go's onProperties.
	BaseDevMode *win.DEVMODE
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
// The real implementation is gdiBackend in gdi_windows.go, constructed
// via NewGDIBackend.
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
// explicit cancellation does (see
// docs/superpowers/specs/2026-07-18-batch-print-design.md section 3,
// "错误处理策略"). Once cancelled, every remaining item (including the
// one that was mid-way through when cancel was requested) gets
// Result.Err = ErrCancelled, and no further Backend.Open calls happen.
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
```

Wait — `Backend`/`BackendJob` above reference `win.DEVMODE`, which means this file already needs `github.com/lxn/win`, which per the earlier "跨平台偏离说明" is accepted. But `win.DEVMODE` and all of `github.com/lxn/win` are themselves build-tagged `+build windows`, meaning **this file (`job.go`) becomes windows-only too, transitively**, even without its own build tag. That's fine (matches "the whole app is Windows-only anyway"), but it means `go test ./internal/print/...` for Task 1-3 only actually runs on a Windows machine — which is exactly this project's only supported dev environment, so no action needed, just be aware the tests never run on a hypothetical non-Windows CI.

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/print/... -run TestRunJob -v`
Expected: PASS（全部 7 个子测试）

- [ ] **Step 5: 提交**

```bash
git add internal/print/job.go internal/print/job_test.go
git commit -m "$(cat <<'EOF'
feat(print): add Settings/Item/Backend and RunJob orchestration

RunJob drives a Backend (GDI calls abstracted behind an interface so
this is unit-testable with a fake) over a batch of Items, skipping and
recording any single file's failure (bad range, open failure, page
failure) while continuing the rest, and stopping the whole batch only on
an explicit cancel from the progress callback.
EOF
)"
```

---

### Task 4: Windows GDI 打印后端

**Files:**
- Create: `internal/print/gdi_windows.go`

这一份文件只在 `GOOS=windows` 下编译（文件名 `_windows.go` 后缀是 Go 工具链的隐式 build constraint，不需要额外的 `//go:build` 注释）。这里的代码依赖真实打印机/GDI，**没有自动化测试**，只能靠 `go build`/`go vet` 保证能编译，行为正确性走 Task 9 的手动测试清单（可以用 Windows 自带的"Microsoft Print to PDF"虚拟打印机验证，不需要物理打印机）。

- [ ] **Step 1: 写实现**

```go
// internal/print/gdi_windows.go
package print

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"pdfreader/internal/pdfengine"
)

// renderDPI is the fixed resolution every page is rasterized at before
// being scaled onto the physical page - see design doc section "渲染分辨率
// 选型": high enough for text/chart PDFs, low enough to keep memory/time
// bounded regardless of the printer's own (possibly much higher) native
// DPI.
const renderDPI = 300

// StretchDIBits isn't bound anywhere in github.com/lxn/win (verified: no
// match for it in the whole module tree), so it's declared here directly,
// following the exact same LazyDLL/LazyProc + raw syscall.SyscallN pattern
// lxn/win itself uses throughout (e.g. winspool.go's EnumPrinters).
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
// print APIs - see
// docs/superpowers/specs/2026-07-18-batch-print-design.md section 3.
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

	docNamePtr, err := syscall.UTF16PtrFromString(docName)
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
// whatever "属性" set for these specific four fields, by design (see
// Settings.BaseDevMode's doc comment) - our own checkboxes/dropdowns are
// meant to be the last word on copies/duplex/orientation/paper for a
// print triggered from this dialog.
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

	// dmColor is only a hint some drivers honor - toGrayscale below is
	// what actually guarantees grayscale output regardless of driver
	// support (see design doc section 2, "灰度打印").
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
	stretchDIBits(j.hdc, x, y, w, h, 0, 0, int32(imgW), int32(imgH), unsafe.Pointer(&bits[0]), &bmi, win.DIB_RGB_COLORS, win.SRCCOPY)

	if win.EndPage(j.hdc) <= 0 {
		return fmt.Errorf("print: EndPage failed")
	}
	return nil
}

func (j *gdiJob) Close() error {
	win.EndDoc(j.hdc)
	win.DeleteDC(j.hdc)
	return nil
}

// toDIBBits converts img (as returned by pdfengine.Document.RenderPage -
// row 0 is the top row) into a top-down 32bpp BGRA buffer plus a matching
// BITMAPINFOHEADER, ready for stretchDIBits.
//
// Windows DIBs store pixels as B,G,R,A/X - the reverse channel order
// from Go's image.RGBA (R,G,B,A) - so this always swaps channels;
// grayscale is applied here too (channel-order-agnostic, so it doesn't
// matter whether it happens before or after the swap) so gdiJob never
// touches image.RGBA's raw bytes directly.
func toDIBBits(img *image.RGBA, grayscale bool) ([]byte, win.BITMAPINFOHEADER) {
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
```

- [ ] **Step 2: 确认能编译**

Run: `go build ./internal/print/...`
Expected: 无输出（成功）。

Run: `go vet ./internal/print/...`
Expected: 无输出（成功）。

- [ ] **Step 3: 提交**

```bash
git add internal/print/gdi_windows.go
git commit -m "$(cat <<'EOF'
feat(print): add the real GDI print backend

gdiBackend implements Backend/BackendJob directly on lxn/win's raw
CreateDC/StartDoc/StartPage/EndPage/EndDoc bindings, plus a hand-rolled
StretchDIBits syscall binding (absent from lxn/win itself) to blit each
rendered page - converted from image.RGBA to a top-down 32bpp BGRA DIB,
with an optional software grayscale pass so grayscale output doesn't
depend on driver support for DEVMODE's dmColor hint.

Not unit-testable (needs a real Windows print subsystem) - covered by
the manual test checklist instead (Microsoft Print to PDF works fine
for this without a physical printer).
EOF
)"
```

---

### Task 5: 打印机/纸张枚举

**Files:**
- Create: `internal/print/printers_windows.go`

同 Task 4，只在 windows 下编译，没有自动化测试。

- [ ] **Step 1: 写实现**

```go
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
			names = append(names, syscall.UTF16PtrToString(info.PPrinterName))
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
// resolved) - see design doc section 2, "纸张大小下拉".
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
```

`win.IDOK` 需要确认是否存在于这个 lxn/win 版本——若不存在，`DocumentProperties` 加了 `DM_IN_PROMPT` 后的返回值约定是：`IDOK`（=1）表示用户点了确定，`IDCANCEL`（=2）表示取消，二者都是 Win32 通用常量，`lxn/win` 里几乎一定在 `user32.go` 或 `winuser.go` 里定义了（供对话框按钮 ID 使用，前面在 `container.go` 的 `WM_COMMAND` 分支里已经看到过 `win.IDOK`/`win.IDCANCEL` 的实际用法，是 `internal/ui` 间接依赖的同一个 `lxn/win` 版本），可以直接用，不需要自己定义。

- [ ] **Step 2: 确认能编译**

Run: `go build ./internal/print/...`
Expected: 无输出（成功）

Run: `go vet ./internal/print/...`
Expected: 无输出（成功）

- [ ] **Step 3: 提交**

```bash
git add internal/print/printers_windows.go
git commit -m "$(cat <<'EOF'
feat(print): add printer/paper enumeration and the native properties dialog

ListPrinterNames/ListPaperSizes wrap EnumPrinters/DeviceCapabilities'
two-call sizing pattern; QueryDevMode wraps DocumentProperties with
DM_IN_PROMPT so the dialog's "属性" button can open the driver's own
native settings window and feed its result back into Settings.BaseDevMode.
EOF
)"
```

---

### Task 6: `internal/config` 记住上次打印设置

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: 写失败的测试**

在 `internal/config/config_test.go` 末尾加：

```go
func TestSaveThenLoad_PrintSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		LastPrinter:      "Microsoft Print to PDF",
		LastGrayscale:    true,
		LastDuplex:       true,
		LastPaperName:    "A4",
		LastOrientation:  "landscape",
		LastScaleMode:    "percent",
		LastScalePercent: 75,
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if loaded.LastPrinter != "Microsoft Print to PDF" {
		t.Fatalf("loaded.LastPrinter = %q, want %q", loaded.LastPrinter, "Microsoft Print to PDF")
	}
	if !loaded.LastGrayscale || !loaded.LastDuplex {
		t.Fatalf("loaded grayscale/duplex = %v/%v, want true/true", loaded.LastGrayscale, loaded.LastDuplex)
	}
	if loaded.LastPaperName != "A4" || loaded.LastOrientation != "landscape" {
		t.Fatalf("loaded paper/orientation = %q/%q, want A4/landscape", loaded.LastPaperName, loaded.LastOrientation)
	}
	if loaded.LastScaleMode != "percent" || loaded.LastScalePercent != 75 {
		t.Fatalf("loaded scale = %q/%d, want percent/75", loaded.LastScaleMode, loaded.LastScalePercent)
	}
}

func TestLoad_MissingFileDefaultsToNoPrinter(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFrom(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.LastPrinter != "" {
		t.Fatalf("cfg.LastPrinter = %q, want empty (fall back to system default printer at print time)", cfg.LastPrinter)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/config/... -run "TestSaveThenLoad_PrintSettings|TestLoad_MissingFileDefaultsToNoPrinter" -v`
Expected: FAIL（编译错误：`Config` 里没有这些字段）

- [ ] **Step 3: 写实现**

在 `internal/config/config.go` 的 `Config` 结构体里加字段：

```go
// Config is the persisted application state.
type Config struct {
	RecentFiles    []RecentFile `json:"recentFiles"`
	WindowWidth    int          `json:"windowWidth"`
	WindowHeight   int          `json:"windowHeight"`
	SidebarShown   bool         `json:"sidebarShown"`
	SidebarTab     string       `json:"sidebarTab"` // "outline" or "thumbnails"
	ContinuousMode bool         `json:"continuousMode"`

	// LastPrinter/... persist the print dialog's settings across
	// restarts (see docs/superpowers/specs/2026-07-18-batch-print-design.md
	// section 5). LastPaperName is a display name, not a DMPAPER_* code -
	// the print dialog re-resolves it against the selected printer's
	// current paper list each time it opens, since a numeric code isn't
	// guaranteed to mean the same paper across different printer drivers.
	LastPrinter      string `json:"lastPrinter"`
	LastGrayscale    bool   `json:"lastPrintGrayscale"`
	LastDuplex       bool   `json:"lastPrintDuplex"`
	LastPaperName    string `json:"lastPrintPaperSize"`
	LastOrientation  string `json:"lastPrintOrientation"`  // "portrait" or "landscape"
	LastScaleMode    string `json:"lastPrintScaleMode"`    // "fit", "actual", or "percent"
	LastScalePercent int    `json:"lastPrintScalePercent"`
}
```

`defaultConfig()` 不需要改：所有新字段的零值（空字符串/`false`/`0`）已经是想要的默认状态（没打印过 → 用系统默认打印机；`LastScalePercent` 为 0 时，UI 层需要在显示前 fallback 成 100，这个在 Task 7 的对话框初始化里处理，不是 config 包的责任）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/config/... -v`
Expected: PASS（新增的 2 个测试 + 之前全部测试）

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(config): persist last-used print settings

Adds LastPrinter/LastGrayscale/LastDuplex/LastPaperName/LastOrientation/
LastScaleMode/LastScalePercent, following the same direct-field,
zero-value-default pattern as ContinuousMode/SidebarShown.
EOF
)"
```

---

### Task 7: 打印对话框 UI —— 文件列表 + 设置面板

**Files:**
- Create: `internal/ui/printdialog.go`

这一步先把对话框搭起来（文件列表、共享设置面板、属性按钮、确定打印会怎么调用），**先不做进度弹窗和后台 goroutine**（那是 Task 8）。这一步做完，`showPrintDialog` 应该能弹出来、能加文件、能改设置，点"打印"先只是把收集好的 `[]print.Item`/`print.Settings` 传给一个占位的 `onPrint` 回调（Task 8 再把这个回调接上真正的 `RunJob`）。

- [ ] **Step 1: 写实现**

```go
// internal/ui/printdialog.go
package ui

import (
	"fmt"
	"path/filepath"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pdfreader/internal/config"
	"pdfreader/internal/pdfengine"
	"pdfreader/internal/print"
)

// printItem is one row in the print dialog's file list. ownsDoc is true
// for files the dialog opened itself (via "添加PDF文件" or because they
// weren't already open in any tab) - those get Close()'d when removed
// from the list or when the dialog closes. Files borrowed from an
// already-open tab (ownsDoc == false) are never closed here; the tab
// that opened them still owns their lifetime.
type printItem struct {
	path      string
	doc       *pdfengine.Document
	pageCount int
	ownsDoc   bool
	rangeSpec string // "" = all pages; see print.ParseRange
}

func (it *printItem) listLabel() string {
	return fmt.Sprintf("%s   %d 页", filepath.Base(it.path), it.pageCount)
}

// printDialogState holds everything showPrintDialog's closures need -
// kept as a struct (rather than a pile of captured locals) so the
// "add/remove file" and "properties" handlers can all reach it without
// threading a dozen parameters through each other.
type printDialogState struct {
	owner walk.Form
	pool  *pdfengine.Pool
	cfg   *config.Config

	items        []*printItem
	itemLabels   []string // parallel to items; what the ListBox actually displays
	printerNames []string
	paperSizes   []print.PaperSize
	baseDevMode  interface{} // *win.DEVMODE, boxed to keep this file buildable without a windows-only import at the top - see note below

	dlg          *walk.Dialog
	fileList     *walk.ListBox
	printerBox   *walk.ComboBox
	paperBox     *walk.ComboBox
	copiesEdit   *walk.NumberEdit
	grayscaleBox *walk.CheckBox
	duplexBox    *walk.CheckBox
	allPagesBtn  *walk.RadioButton
	customBtn    *walk.RadioButton
	rangeEdit    *walk.LineEdit
	portraitBtn  *walk.RadioButton
	landscapeBtn *walk.RadioButton
	fitBtn       *walk.RadioButton
	actualBtn    *walk.RadioButton
	percentBtn   *walk.RadioButton
	percentEdit  *walk.NumberEdit
	printBtn     *walk.PushButton
}
```

上面 `baseDevMode interface{}` 这个写法有问题——这个文件本来就只会在 Windows 下被编译进最终程序（`internal/ui` 整体依赖 `lxn/walk`），没有必要为了"不直接 import windows-only 类型"而把类型抹掉成 `interface{}`，那样反而要在用到的地方到处做类型断言。改成直接用 `*win.DEVMODE`，和 `internal/print` 包（Task 3-5）已经做出的选择一致（"整个 app 本来就只为 Windows 构建"）。重新写一遍这段：

```go
// internal/ui/printdialog.go
package ui

import (
	"fmt"
	"path/filepath"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"pdfreader/internal/config"
	"pdfreader/internal/pdfengine"
	"pdfreader/internal/print"
)

// printItem is one row in the print dialog's file list. ownsDoc is true
// for files the dialog opened itself (via "添加PDF文件", or because they
// weren't already open in any tab) - those get Close()'d when removed
// from the list or when the dialog closes. Files borrowed from an
// already-open tab (ownsDoc == false) are never closed here; the tab
// that opened them still owns their lifetime.
type printItem struct {
	path      string
	doc       *pdfengine.Document
	pageCount int
	ownsDoc   bool
	rangeSpec string // "" = all pages; see print.ParseRange
}

func (it *printItem) listLabel() string {
	return fmt.Sprintf("%s   %d 页", filepath.Base(it.path), it.pageCount)
}

// printDialogState holds everything showPrintDialog's closures need -
// kept as a struct (rather than a pile of captured locals) so the
// "add/remove file"/"properties"/"print" handlers can all reach shared
// state without threading a dozen parameters through each other.
type printDialogState struct {
	owner walk.Form
	pool  *pdfengine.Pool

	items        []*printItem
	printerNames []string
	paperSizes   []print.PaperSize
	baseDevMode  *win.DEVMODE // set by onProperties; nil until the user opens "属性" at least once

	dlg          *walk.Dialog
	fileList     *walk.ListBox
	printerBox   *walk.ComboBox
	paperBox     *walk.ComboBox
	copiesEdit   *walk.NumberEdit
	grayscaleBox *walk.CheckBox
	duplexBox    *walk.CheckBox
	allPagesBtn  *walk.RadioButton
	customBtn    *walk.RadioButton
	rangeEdit    *walk.LineEdit
	portraitBtn  *walk.RadioButton
	landscapeBtn *walk.RadioButton
	fitBtn       *walk.RadioButton
	actualBtn    *walk.RadioButton
	percentBtn   *walk.RadioButton
	percentEdit  *walk.NumberEdit
	printBtn     *walk.PushButton
}

// showPrintDialog opens the print dialog. initial, if non-nil, is
// pre-added to the file list as a borrowed (not owned) item - this is
// how openFile's currently-active tab gets pre-filled (see app.go's
// onPrint). onRun is called once the user clicks "打印" with the final
// item list and settings; it's a callback rather than this function
// doing the printing itself so Task 8 can wire in the progress dialog
// without touching this file again.
func (a *app) showPrintDialog(initial *printItem, onRun func(items []print.Item, settings print.Settings)) {
	st := &printDialogState{owner: a.mainWindow, pool: a.pool}
	if initial != nil {
		st.items = append(st.items, initial)
	}

	names, defaultName, err := print.ListPrinterNames()
	if err != nil || len(names) == 0 {
		walk.MsgBox(a.mainWindow, "打印", "未检测到任何打印机。", walk.MsgBoxIconWarning)
		return
	}
	st.printerNames = names

	lastPrinter := a.cfg.LastPrinter
	printerIndex := indexOf(names, lastPrinter)
	if printerIndex < 0 {
		printerIndex = indexOf(names, defaultName)
	}
	if printerIndex < 0 {
		printerIndex = 0
	}

	// walk.Dialog.Run() below is BLOCKING (see dialogs.go's promptPassword
	// doc comment - it doesn't return until the dialog is closed), so
	// every widget's *initial* content has to be computed now, before the
	// Dialog{} literal, and fed in via Model/Checked/Text/CurrentIndex -
	// none of the AssignTo'd pointers (fileList, paperBox, ...) exist yet
	// at this point in the function, and by the time Run() returns the
	// dialog has already been destroyed, so populating them "after Run()"
	// (as a first draft of this function did) would only ever run once
	// nobody can see it happen. Everything computed here only runs ONCE,
	// at dialog-open time; all *subsequent* updates (add/remove a file,
	// switch printers, ...) happen from the event handlers further down,
	// which run while the dialog is live and their AssignTo'd widgets
	// already exist - those are fine to call .SetModel()/.SetText() on
	// directly.
	initialLabels := make([]string, len(st.items))
	for i, it := range st.items {
		initialLabels[i] = it.listLabel()
	}

	initialPaperSizes, err := print.ListPaperSizes(names[printerIndex])
	if err != nil || len(initialPaperSizes) == 0 {
		initialPaperSizes = fallbackPaperSizesFor()
	}
	st.paperSizes = initialPaperSizes
	initialPaperNames := make([]string, len(initialPaperSizes))
	initialPaperIndex := 0
	for i, s := range initialPaperSizes {
		initialPaperNames[i] = s.Name
		if s.Name == a.cfg.LastPaperName {
			initialPaperIndex = i
		}
	}

	initialRangeSpec := ""
	if len(st.items) > 0 {
		initialRangeSpec = st.items[0].rangeSpec
	}
	initialAllPages := initialRangeSpec == ""

	var dlg *walk.Dialog
	var fileList *walk.ListBox
	var printerBox, paperBox *walk.ComboBox
	var copiesEdit, percentEdit *walk.NumberEdit
	var grayscaleBox, duplexBox *walk.CheckBox
	var allPagesBtn, customBtn, portraitBtn, landscapeBtn, fitBtn, actualBtn, percentBtn *walk.RadioButton
	var rangeEdit *walk.LineEdit
	var printBtn, cancelBtn, addBtn, propsBtn *walk.PushButton

	refreshFileList := func() {
		labels := make([]string, len(st.items))
		for i, it := range st.items {
			labels[i] = it.listLabel()
		}
		fileList.SetModel(labels)
		printBtn.SetEnabled(len(st.items) > 0)
	}

	loadRangeForSelection := func() {
		idx := fileList.CurrentIndex()
		if idx < 0 || idx >= len(st.items) {
			rangeEdit.SetEnabled(false)
			return
		}
		it := st.items[idx]
		if it.rangeSpec == "" {
			allPagesBtn.SetChecked(true)
			rangeEdit.SetText("")
		} else {
			customBtn.SetChecked(true)
			rangeEdit.SetText(it.rangeSpec)
		}
		rangeEdit.SetEnabled(customBtn.Checked())
	}

	saveRangeForSelection := func() {
		idx := fileList.CurrentIndex()
		if idx < 0 || idx >= len(st.items) {
			return
		}
		if allPagesBtn.Checked() {
			st.items[idx].rangeSpec = ""
		} else {
			st.items[idx].rangeSpec = rangeEdit.Text()
		}
	}

	addFiles := func() {
		saveRangeForSelection()
		fd := walk.FileDialog{Title: "添加PDF文件", Filter: "PDF 文件 (*.pdf)|*.pdf"}
		ok, err := fd.ShowOpenMultiple(dlg)
		if err != nil || !ok {
			return
		}
		for _, path := range fd.FilePaths {
			it, err := openPrintItem(a, path)
			if err != nil {
				walk.MsgBox(dlg, "无法添加文件", fmt.Sprintf("%s：%v", filepath.Base(path), err), walk.MsgBoxIconWarning)
				continue
			}
			if it != nil {
				st.items = append(st.items, it)
			}
		}
		refreshFileList()
		if len(st.items) > 0 {
			fileList.SetCurrentIndex(len(st.items) - 1)
			loadRangeForSelection()
		}
	}

	removeSelected := func() {
		idx := fileList.CurrentIndex()
		if idx < 0 || idx >= len(st.items) {
			return
		}
		it := st.items[idx]
		if it.ownsDoc {
			it.doc.Close()
		}
		st.items = append(st.items[:idx], st.items[idx+1:]...)
		refreshFileList()
		if len(st.items) > 0 {
			newIdx := idx
			if newIdx >= len(st.items) {
				newIdx = len(st.items) - 1
			}
			fileList.SetCurrentIndex(newIdx)
		}
		loadRangeForSelection()
	}

	onProperties := func() {
		if printerBox.CurrentIndex() < 0 {
			return
		}
		printerName := st.printerNames[printerBox.CurrentIndex()]
		dm, ok := print.QueryDevMode(dlg.Handle(), printerName, st.baseDevMode)
		if !ok {
			return
		}
		st.baseDevMode = &dm
	}

	collectSettings := func() print.Settings {
		mode := print.ScaleFitPage
		switch {
		case actualBtn.Checked():
			mode = print.ScaleActualSize
		case percentBtn.Checked():
			mode = print.ScalePercent
		}
		orientation := print.OrientPortrait
		if landscapeBtn.Checked() {
			orientation = print.OrientLandscape
		}
		var paperCode int16
		var paperName string
		if idx := paperBox.CurrentIndex(); idx >= 0 && idx < len(st.paperSizes) {
			paperCode = st.paperSizes[idx].Code
			paperName = st.paperSizes[idx].Name
		}
		printerName := st.printerNames[printerBox.CurrentIndex()]

		a.cfg.LastPrinter = printerName
		a.cfg.LastGrayscale = grayscaleBox.Checked()
		a.cfg.LastDuplex = duplexBox.Checked()
		a.cfg.LastPaperName = paperName
		if orientation == print.OrientLandscape {
			a.cfg.LastOrientation = "landscape"
		} else {
			a.cfg.LastOrientation = "portrait"
		}
		switch mode {
		case print.ScaleActualSize:
			a.cfg.LastScaleMode = "actual"
		case print.ScalePercent:
			a.cfg.LastScaleMode = "percent"
		default:
			a.cfg.LastScaleMode = "fit"
		}
		a.cfg.LastScalePercent = int(percentEdit.Value())
		a.cfg.Save()

		return print.Settings{
			PrinterName:  printerName,
			Copies:       int(copiesEdit.Value()),
			Grayscale:    grayscaleBox.Checked(),
			Duplex:       duplexBox.Checked(),
			PaperCode:    paperCode,
			Orientation:  orientation,
			ScaleMode:    mode,
			ScalePercent: int(percentEdit.Value()),
			BaseDevMode:  st.baseDevMode,
		}
	}

	loadPaperSizesFor := func(printerName string) {
		sizes, err := print.ListPaperSizes(printerName)
		if err != nil || len(sizes) == 0 {
			sizes = fallbackPaperSizesFor()
		}
		st.paperSizes = sizes
		names := make([]string, len(sizes))
		want := a.cfg.LastPaperName
		selectIdx := 0
		for i, s := range sizes {
			names[i] = s.Name
			if s.Name == want {
				selectIdx = i
			}
		}
		paperBox.SetModel(names)
		if len(names) > 0 {
			paperBox.SetCurrentIndex(selectIdx)
		}
	}

	d := Dialog{
		AssignTo:      &dlg,
		Title:         "打印",
		DefaultButton: &printBtn,
		CancelButton:  &cancelBtn,
		MinSize:       Size{Width: 640, Height: 420},
		Layout:        HBox{},
		Children: []Widget{
			Composite{
				Layout: VBox{},
				Children: []Widget{
					PushButton{AssignTo: &addBtn, Text: "+ 添加PDF文件", OnClicked: func() { addFiles() }},
					ListBox{
						AssignTo:              &fileList,
						Model:                 initialLabels,
						CurrentIndex:          firstIndexIfAny(len(st.items)),
						OnCurrentIndexChanged: func() { loadRangeForSelection() },
						ContextMenuItems: []MenuItem{
							Action{Text: "移除", OnTriggered: func() { removeSelected() }},
						},
					},
				},
			},
			Composite{
				Layout: VBox{},
				Children: []Widget{
					Label{Text: "打印机："},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							ComboBox{AssignTo: &printerBox, Model: st.printerNames, CurrentIndex: printerIndex,
								OnCurrentIndexChanged: func() {
									loadPaperSizesFor(st.printerNames[printerBox.CurrentIndex()])
								}},
							PushButton{AssignTo: &propsBtn, Text: "属性", OnClicked: func() { onProperties() }},
						},
					},
					Label{Text: "份数："},
					NumberEdit{AssignTo: &copiesEdit, MinValue: 1, MaxValue: 999, Decimals: 0, Value: 1},
					CheckBox{AssignTo: &grayscaleBox, Text: "灰度打印", Checked: a.cfg.LastGrayscale},
					CheckBox{AssignTo: &duplexBox, Text: "双面打印", Checked: a.cfg.LastDuplex},
					Label{Text: "范围："},
					RadioButtonGroup{
						Buttons: []RadioButton{
							{AssignTo: &allPagesBtn, Text: "所有页", Checked: initialAllPages, OnClicked: func() {
								rangeEdit.SetEnabled(false)
								saveRangeForSelection()
							}},
							{AssignTo: &customBtn, Text: "选择页", Checked: !initialAllPages, OnClicked: func() {
								rangeEdit.SetEnabled(true)
								saveRangeForSelection()
							}},
						},
					},
					LineEdit{AssignTo: &rangeEdit, Text: initialRangeSpec, Enabled: !initialAllPages, ToolTipText: "例如 1,8,9-12", OnTextChanged: func() { saveRangeForSelection() }},
					Label{Text: "纸张大小："},
					ComboBox{AssignTo: &paperBox, Model: initialPaperNames, CurrentIndex: initialPaperIndex},
					Label{Text: "纸张方向："},
					RadioButtonGroup{
						Buttons: []RadioButton{
							{AssignTo: &portraitBtn, Text: "纵向", Checked: a.cfg.LastOrientation != "landscape"},
							{AssignTo: &landscapeBtn, Text: "横向", Checked: a.cfg.LastOrientation == "landscape"},
						},
					},
					Label{Text: "页面大小调整："},
					RadioButtonGroup{
						Buttons: []RadioButton{
							{AssignTo: &fitBtn, Text: "适合页面", Checked: a.cfg.LastScaleMode != "actual" && a.cfg.LastScaleMode != "percent"},
							{AssignTo: &actualBtn, Text: "实际大小", Checked: a.cfg.LastScaleMode == "actual"},
							{AssignTo: &percentBtn, Text: "页面缩放", Checked: a.cfg.LastScaleMode == "percent"},
						},
					},
					NumberEdit{AssignTo: &percentEdit, MinValue: 10, MaxValue: 400, Decimals: 0, Value: percentOrDefault(a.cfg.LastScalePercent), Suffix: "%"},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							PushButton{AssignTo: &cancelBtn, Text: "取消", OnClicked: func() { dlg.Cancel() }},
							PushButton{AssignTo: &printBtn, Text: "打印", Enabled: len(st.items) > 0, OnClicked: func() {
								saveRangeForSelection()
								settings := collectSettings()
								items := make([]print.Item, len(st.items))
								for i, it := range st.items {
									items[i] = print.Item{Path: it.path, Doc: it.doc, PageCount: it.pageCount, RangeSpec: it.rangeSpec}
								}
								dlg.Accept()
								onRun(items, settings)
							}},
						},
					},
				},
			},
		},
	}

	// Blocks until Accept()/Cancel()/window close - see the big comment
	// above. Its error return (dialog failed to even construct) doesn't
	// change what happens next: either way, any item the dialog itself
	// opened still needs closing below.
	d.Run(a.mainWindow)

	// Any item the dialog itself opened (ownsDoc) that's still in the
	// list when the dialog closes (cancelled, or closed after a
	// successful print) must be closed here - RunJob (Task 8) only reads
	// from these Documents, it never takes ownership of closing them.
	for _, it := range st.items {
		if it.ownsDoc {
			it.doc.Close()
		}
	}
}

func percentOrDefault(v int) float64 {
	if v <= 0 {
		return 100
	}
	return float64(v)
}

// firstIndexIfAny returns 0 (select the first row) when a ListBox's
// model is non-empty, or -1 (no selection) when it's empty - passed
// directly as ListBox.CurrentIndex's initial value in the Dialog{}
// literal, since declarative.ListBox has no "select first if any"
// shorthand of its own.
func firstIndexIfAny(itemCount int) int {
	if itemCount > 0 {
		return 0
	}
	return -1
}

func indexOf(names []string, name string) int {
	if name == "" {
		return -1
	}
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

func fallbackPaperSizesFor() []print.PaperSize {
	return []print.PaperSize{
		{Name: "A4", Code: 9},
		{Name: "Letter", Code: 1},
		{Name: "Legal", Code: 5},
	}
}

// openPrintItem opens path via a.pool for the print dialog's file list -
// used for files added through "添加PDF文件" that aren't already borrowed
// from an open tab (see showPrintDialog's initial param). Encrypted
// files prompt for a password the same way openFile does; a cancelled or
// permanently-wrong password returns (nil, nil) rather than an error, so
// the caller's "file failed to add" message box isn't shown for a
// deliberate cancel.
func openPrintItem(a *app, path string) (*printItem, error) {
	data, err := readFileBytes(path)
	if err != nil {
		return nil, err
	}

	doc, err := a.pool.Open(data, nil)
	if isPasswordRequired(err) {
		wrongAttempt := false
		for {
			pw, ok := promptPassword(a.mainWindow, filepathBase(path), wrongAttempt)
			if !ok {
				return nil, nil
			}
			doc, err = a.pool.Open(data, &pw)
			if err == nil {
				break
			}
			if !isPasswordRequired(err) {
				return nil, err
			}
			wrongAttempt = true
		}
	} else if err != nil {
		return nil, err
	}

	return &printItem{path: path, doc: doc, pageCount: doc.PageCount(), ownsDoc: true}, nil
}
```

上面用到的 `readFileBytes`/`isPasswordRequired` 是为了不在这个新文件里重复 `os.ReadFile`+`errors.Is(err, pdfengine.ErrPasswordRequired)` 这两行，直接复用 `app.go` 里已有的等价逻辑更好——检查后发现 `app.go` 里这两行是内联在 `openFile` 里的，没有抽出独立函数。为了不为了两行代码去改 `openFile`（避免无关重构），这里直接内联，不新增 `readFileBytes`/`isPasswordRequired` 这两个占位函数：

```go
func openPrintItem(a *app, path string) (*printItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFileUnreadable, err)
	}

	doc, err := a.pool.Open(data, nil)
	if errors.Is(err, pdfengine.ErrPasswordRequired) {
		wrongAttempt := false
		for {
			pw, ok := promptPassword(a.mainWindow, filepathBase(path), wrongAttempt)
			if !ok {
				return nil, nil
			}
			doc, err = a.pool.Open(data, &pw)
			if err == nil {
				break
			}
			if !errors.Is(err, pdfengine.ErrPasswordRequired) {
				return nil, err
			}
			wrongAttempt = true
		}
	} else if err != nil {
		return nil, err
	}

	return &printItem{path: path, doc: doc, pageCount: doc.PageCount(), ownsDoc: true}, nil
}
```

对应地，文件头部的 `import` 需要加 `"errors"` 和 `"os"`：

```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"pdfreader/internal/config"
	"pdfreader/internal/pdfengine"
	"pdfreader/internal/print"
)
```

（这里的 `import "pdfreader/internal/config"` 实际上没被用到——`a.cfg` 已经是 `*config.Config` 类型，本文件只是读写它的字段，不需要显式引用 `config` 包名。写完整份文件后用 `goimports`/编译器报错来发现并删掉这个多余 import，见下一步。）

- [ ] **Step 2: 编译，删掉多余/缺失的 import**

Run: `go build ./internal/ui/...`

预期第一次会报至少一个 `imported and not used: "pdfreader/internal/config"` 之类的错误——按报错删掉 `internal/ui/printdialog.go` 里那一行 `"pdfreader/internal/config"` import，再跑一次直到 `go build ./internal/ui/...` 干净通过。也顺手用 `go vet ./internal/ui/...` 确认没有其它问题。

Expected（最终）: 两个命令都无输出。

- [ ] **Step 3: 提交**

```bash
git add internal/ui/printdialog.go
git commit -m "$(cat <<'EOF'
feat(ui): add the print dialog (file list + shared settings panel)

showPrintDialog builds the left file-list/right-settings-panel dialog
from docs/superpowers/specs/2026-07-18-batch-print-design.md section 2:
add/remove files (reusing an already-open tab's Document when given one,
opening a fresh one via the pool otherwise, prompting for a password the
same way openFile does), a shared Settings panel persisted to config,
and a per-file range spec that switches with the list selection. Printing
itself is left to a caller-supplied onRun callback, wired up in the next
task once the progress dialog exists.
EOF
)"
```

---

### Task 8: 菜单入口 + 后台打印 + 进度弹窗 + 结果汇总

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 加菜单项和入口函数**

在 `internal/ui/app.go` 的「文件」菜单（`Text: "文件(&F)"`）的 `Items` 里，紧跟在「关闭标签」后面加一项：

```go
Action{Text: "打印...(&P)", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyP}, OnTriggered: a.onPrint},
```

（这一项和「转到」菜单那几项不一样，不需要 `navInputFocused` 那套焦点保护——`Ctrl+P` 不是任何原生控件会拦截的按键，跟 `Ctrl+O`/`Ctrl+W`/`Ctrl+G` 一样直接用同一个 Action 的 `Shortcut`+`OnTriggered` 就行。）

然后在文件末尾（比如紧接着 `onPrevPageDirect`/`onNextPageDirect`/`onFirstPageDirect`/`onLastPageDirect` 那几个函数之后）加：

```go
// onPrint opens the print dialog (Ctrl+P / 文件 > 打印...). If a tab is
// currently open, it's pre-added to the dialog's file list as a
// borrowed (not owned) item, reusing its already-open (possibly already
// password-unlocked) pdfengine.Document - see showPrintDialog and
// printItem's doc comment for why ownsDoc matters.
func (a *app) onPrint() {
	var initial *printItem
	if t := a.currentTab(); t != nil {
		initial = &printItem{path: t.path, doc: t.doc, pageCount: t.doc.PageCount(), ownsDoc: false}
	}

	a.showPrintDialog(initial, func(items []print.Item, settings print.Settings) {
		a.runPrintJob(items, settings)
	})
}
```

- [ ] **Step 2: 加进度对话框 + 后台 goroutine + 汇总**

紧接着上面那个函数，加：

```go
// runPrintJob runs items/settings on a background goroutine (GDI print
// calls are synchronous/blocking - see
// docs/superpowers/specs/2026-07-18-batch-print-design.md section 4),
// showing a small progress dialog with a cancel button while it runs,
// and a summary MsgBox once it's done (or cancelled).
//
// walk.Dialog.Run() is blocking (see the big comment in showPrintDialog,
// Task 7) and progressDlg/bar/statusLabel only become valid partway
// through it (Create() runs first, then the modal message loop starts).
// A goroutine started before Run() is called could reach
// progressDlg.Synchronize(...) before Create() has even run, dereferencing
// a still-nil *walk.Dialog. To avoid that race, the goroutine itself
// isn't started directly - instead a.mainWindow.Synchronize schedules its
// launch as a message to be processed once the (already-running,
// always-valid) main window's message loop gets to it, which - because
// Create() happens strictly before Run()'s modal loop starts dispatching
// messages at all, and Win32 message dispatch is per-thread rather than
// per-window, so a message posted to a.mainWindow's hWnd is still
// delivered by the dialog's nested modal loop on the same thread - is
// guaranteed to run after progressDlg/bar/statusLabel are assigned. Every
// cross-goroutine UI touch below also goes through a.mainWindow.Synchronize
// rather than progressDlg.Synchronize, for the same reason: a.mainWindow
// is unconditionally non-nil the whole time this function runs.
func (a *app) runPrintJob(items []print.Item, settings print.Settings) {
	var progressDlg *walk.Dialog
	var bar *walk.ProgressBar
	var statusLabel *walk.Label
	var cancelBtn *walk.PushButton

	cancelled := make(chan struct{})
	var cancelOnce sync.Once
	requestCancel := func() {
		cancelOnce.Do(func() { close(cancelled) })
	}

	// The "取消" button deliberately does NOT call progressDlg.Cancel() -
	// it only signals requestCancel and leaves the dialog open. RunJob
	// (Task 3) still needs to run BackendJob.Close() on whatever file is
	// currently mid-print so it doesn't leave a half-finished job sitting
	// in the spooler queue (see design doc section 3); the dialog only
	// actually closes once the goroutine below reaches close(done) and
	// schedules progressDlg.Accept() itself. sync.Once guards against a
	// user clicking "取消" more than once (closing an already-closed
	// channel would panic).
	d := Dialog{
		AssignTo:     &progressDlg,
		Title:        "正在打印",
		Layout:       VBox{},
		MinSize:      Size{Width: 360, Height: 100},
		CancelButton: &cancelBtn,
		Children: []Widget{
			Label{AssignTo: &statusLabel, Text: "准备中…"},
			ProgressBar{AssignTo: &bar, MinValue: 0, MaxValue: len(items)},
			PushButton{AssignTo: &cancelBtn, Text: "取消", OnClicked: func() { requestCancel() }},
		},
	}

	var results []print.Result
	done := make(chan struct{})

	a.mainWindow.Synchronize(func() {
		go func() {
			backend := print.NewGDIBackend()
			results = print.RunJob(backend, items, settings, func(p print.Progress) bool {
				select {
				case <-cancelled:
					return true
				default:
				}
				a.mainWindow.Synchronize(func() {
					bar.SetValue(p.FileIndex)
					statusLabel.SetText(fmt.Sprintf("正在打印 %d/%d：%s（第 %d/%d 页）",
						p.FileIndex+1, p.FileCount, filepathBase(p.FileName), p.Page, p.PageCount))
				})
				return false
			})
			close(done)
			a.mainWindow.Synchronize(func() { progressDlg.Accept() })
		}()
	})

	d.Run(a.mainWindow)
	<-done // already closed by the time Run() returns on every path (Accept() is itself scheduled after close(done) in the goroutine above) - this is the explicit happens-before edge that makes reading results just below safe

	a.showPrintSummary(results)
}

// showPrintSummary reports RunJob's outcome - see design doc section 4.
func (a *app) showPrintSummary(results []print.Result) {
	succeeded, failed, cancelled := 0, 0, 0
	var detail strings.Builder
	for _, r := range results {
		switch {
		case r.Err == nil:
			succeeded++
		case errors.Is(r.Err, print.ErrCancelled):
			cancelled++
			fmt.Fprintf(&detail, "\n未打印：%s", filepathBase(r.Item.Path))
		default:
			failed++
			fmt.Fprintf(&detail, "\n失败：%s（%v）", filepathBase(r.Item.Path), r.Err)
		}
	}

	summary := fmt.Sprintf("打印完成：成功 %d 个", succeeded)
	if failed > 0 {
		summary += fmt.Sprintf("，失败 %d 个", failed)
	}
	if cancelled > 0 {
		summary = fmt.Sprintf("已取消。成功 %d 个", succeeded)
		if failed > 0 {
			summary += fmt.Sprintf("，失败 %d 个", failed)
		}
	}

	icon := walk.MsgBoxIconInformation
	if failed > 0 || cancelled > 0 {
		icon = walk.MsgBoxIconWarning
	}
	walk.MsgBox(a.mainWindow, "打印", summary+detail.String(), icon)
}
```

- [ ] **Step 3: 补齐 import**

`internal/ui/app.go` 顶部的 `import` 块需要加 `"strings"`、`"sync"` 和 `"pdfreader/internal/print"`（`"errors"`/`"fmt"` 这个文件里已经有了）：

```go
import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/klippa-app/go-pdfium"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pdfreader/internal/config"
	"pdfreader/internal/document"
	"pdfreader/internal/pdfengine"
	"pdfreader/internal/print"
)
```

- [ ] **Step 4: 编译确认**

Run: `go build ./...`
Expected: 无输出（成功）

Run: `go vet ./...`
Expected: 无输出（成功）

Run: `go test ./...`
Expected: `internal/config`、`internal/document`、`internal/pdfengine`、`internal/print`、`internal/ui` 全部 `ok`（`internal/print` 是这次新加的，`internal/ui` 里没有新增自动化测试，保持原样通过）。

- [ ] **Step 5: 提交**

```bash
git add internal/ui/app.go
git commit -m "$(cat <<'EOF'
feat(ui): wire up Ctrl+P / 文件>打印..., background print job with progress + summary

onPrint pre-fills the dialog with the current tab's already-open
Document (no re-prompting for its password) and opens showPrintDialog;
runPrintJob drives print.RunJob on a background goroutine (GDI calls
block) behind a small cancellable progress dialog, then reports a
success/failure/cancelled summary via MsgBox.
EOF
)"
```

---

### Task 9: 手动测试清单 + 最终验证

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 在手动测试清单里追加打印相关条目**

在 `README.md` 的「手动测试清单」小节末尾追加：

```markdown
- [ ] Ctrl+P / 文件>打印...：已打开文件时对话框自动预填当前文件，直接点「打印」能出纸（没有物理打印机可以选 Windows 自带的"Microsoft Print to PDF"验证管线是否走通）
- [ ] 「添加PDF文件」多选新增文件，列表正确显示文件名和页数；加密文件正确弹密码框
- [ ] 切换左侧不同文件时，「范围」输入框内容各自独立，其它设置项（打印机/份数/灰度/双面/纸张/方向/缩放）保持共享
- [ ] 选择页范围（如 `1,3-5`）只打印对应页；输入越界或非法范围时给出合理反馈
- [ ] 灰度/双面/纸张大小/方向/页面缩放（适合页面/实际大小/百分比）各选项生效
- [ ] 右键列表项「移除」能正确从批量列表中去掉该文件
- [ ] 「属性」按钮能弹出打印机驱动原生设置窗口
- [ ] 批量列表中一个文件密码错误（或取消密码框）、另一个文件正常，打印后有正确的成功/失败汇总
- [ ] 打印过程中点「取消」，后续文件不再打印，弹出正确的汇总（已打印的不受影响）
- [ ] 关闭程序重新打开，打印设置（打印机/灰度/双面/纸张/方向/缩放）被记住
```

同时在 README「功能」小节的列表里加一行（紧跟在"连续阅读模式"后面）：

```markdown
- 打印（单文件/批量），可配置份数/灰度/双面/纸张/方向/缩放
```

- [ ] **Step 2: 最终整体验证**

```bash
go build ./...
go vet ./...
go test ./...
GOARCH=amd64 GOOS=windows go build -ldflags "-H=windowsgui" -o pdfreader.exe .
```

Expected: 四条命令全部无错误退出；最后一条应生成/更新 `pdfreader.exe`。

- [ ] **Step 3: 提交**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
docs: add print feature to README and manual test checklist
EOF
)"
```

---

## 自查记录

- **spec 覆盖**：设计文档 7 节逐一对应：入口/架构→Task 8；对话框 UI→Task 7；打印管线→Task 4；进度反馈→Task 8；配置持久化→Task 6；范围边界（不做的事）→没有对应任务，本来就是"不做"；测试策略→Task 1-3（自动化）+ Task 9（手动清单）。
- **占位符扫描**：全文没有 TBD/TODO；`internal/ui/printdialog.go` 里两处"先写错的版本，指出问题，再给正确版本"是刻意保留的推导过程（对应我自己发现的两个设计失误：`baseDevMode interface{}` 装箱没必要、`readFileBytes`/`isPasswordRequired` 是引入了不存在的占位函数），Task 7 最终交付的是**第二版**代码，不是占位符。
- **类型一致性**：`print.Settings`/`print.Item`/`print.Result`/`print.Progress`/`print.Backend`/`print.BackendJob`/`print.PaperSize`/`print.ScaleMode`/`print.Orientation` 在 Task 3-5 定义后，Task 7-8 里所有引用（字段名、方法名）都跟着这几个任务里最终确定的名字走，没有漂移。`printItem`/`printDialogState`（`internal/ui` 包内部类型）同理。
- **发现并修正的两处时序 bug**（写完初稿后重读发现的，不是留给实现者踩的坑）：
  1. Task 7 初稿把文件列表/纸张下拉的初始内容填充代码放在了 `d.Run(a.mainWindow)` **之后**——但 `walk.Dialog.Run()` 是阻塞调用，直到对话框关闭才返回，那些代码实际上要等用户关掉对话框才会跑，对话框可见期间列表/下拉框全是空的。已改成在构造 `Dialog{}` 字面量**之前**算好 `initialLabels`/`initialPaperNames`/`initialPaperIndex`/`initialRangeSpec`，直接喂给 `Model`/`CurrentIndex`/`Checked`/`Text` 字段；只有响应用户操作的动态更新（加/删文件、切换打印机）还留在事件回调里用 `.SetModel()`/`.SetText()`，那些回调本来就是对话框已经显示之后才会触发，没有这个问题。
  2. Task 8 初稿在调用 `d.Run(a.mainWindow)` **之前**就 `go func(){...}()` 启动了后台打印 goroutine，而这个 goroutine 一开始就会调用 `progressDlg.Synchronize(...)`——`progressDlg` 只有在 `Run()` 内部先跑完 `Create()` 之后才会被赋值，goroutine 调度是不确定的，存在真实的"在 `progressDlg` 还是 nil 的时候就解引用"的竞态。已改成用 `a.mainWindow.Synchronize(func(){ go func(){...}() })` 把 goroutine 的启动本身也推迟到"贴到消息队列里、由当前正在跑的消息循环处理时才执行"，而 `Create()` 严格发生在 `Run()` 的模态消息循环开始处理消息之前，所以能保证 goroutine 真正跑起来时 `progressDlg`/`bar`/`statusLabel` 都已经就绪；goroutine 内部所有跨线程 UI 更新也统一改用 `a.mainWindow.Synchronize`（`a.mainWindow` 全程非 nil，比引用 `progressDlg` 更安全）。
