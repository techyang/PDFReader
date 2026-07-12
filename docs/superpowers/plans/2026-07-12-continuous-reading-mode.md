# 连续阅读模式实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有单页阅读模式之外加一个可切换的"连续阅读模式"——垂直滚动浏览整份文档，只渲染视口内可见的页面，全局持久化到配置。

**Architecture:** `splitter → pageScroll(walk.ScrollView) → pageView(walk.CustomWidget)` 替换现在的 `splitter → pageView`，单页/连续两种模式共用这棵控件树。纯几何计算（每页在连续模式下的位置、可见页范围、"哪页占比最大"）放进 `internal/document`（可单测），`internal/ui` 只负责用这些计算结果去调用 `pdfengine.RenderPage`、摆放位图、驱动滚动条。

**Tech Stack:** 沿用现有的 `github.com/lxn/walk` + `walk/declarative`；新增直接依赖 `github.com/lxn/win`（已经是 walk 的间接依赖，用来在程序化跳转页码时同步原生滚动条滑块位置，因为 `walk.ScrollView` 没有公开的"滚动到指定位置"方法）。

**依赖的前置设计文档：** `docs/superpowers/specs/2026-07-12-continuous-reading-mode-design.md`（已经过 brainstorming 确认）。

---

## 开始前须知：先提交现存的未提交改动

在开始 Task 1 之前，工作区里已经有三块跟这次连续阅读模式无关、但还没提交的改动，应该先分别提交，避免和这次的一堆新提交混在一起、不好 review：

1. `internal/ui/app.go` 里两处已经验证过的 bugfix（工具栏 `ButtonStyle` 导致按钮空白；侧边栏没设置拉伸比例导致占一半宽度）。
2. `README.md`（Task 21：构建/运行说明 + 手动测试清单，包含 `-H=windowsgui` 的编译参数说明）。
3. 两个空的 `scratch_invalid.log`/`scratch_valid.log` 文件——之前测试遗留的，不属于任何提交，应该删掉。

- [ ] **Step 0: 分别提交前置改动，清理垃圾文件**

```bash
cd "E:/workspace_go/PDFReader"
rm scratch_invalid.log scratch_valid.log
git add internal/ui/app.go
git commit -m "fix: toolbar buttons render blank without ButtonStyle; sidebar takes half the width without a stretch factor"
git add README.md
git commit -m "docs: add build instructions and manual test checklist"
```

Expected: 两次提交成功，`git status` 里只剩这次计划要新增的文件（如果已经跑到这一步之后又建了新文件的话）。

---

### Task 1: document.LayoutContinuous — 连续模式每页布局计算

**Files:**
- Create: `internal/document/continuous.go`
- Test: `internal/document/continuous_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/document/continuous_test.go
package document

import (
	"math"
	"testing"
)

func TestLayoutContinuous_SinglePageFitWidth(t *testing.T) {
	sizes := [][2]float64{{200, 400}} // 200x400pt page
	zoom := Zoom{Mode: ZoomFitWidth}
	layouts, total := LayoutContinuous(sizes, zoom, 800, 8)

	if len(layouts) != 1 {
		t.Fatalf("len(layouts) = %d, want 1", len(layouts))
	}
	l := layouts[0]
	if l.Top != 0 {
		t.Fatalf("layouts[0].Top = %v, want 0", l.Top)
	}
	wantScale := 800.0 / 200.0
	wantDPI := DPIForScale(wantScale)
	if l.DPI != wantDPI {
		t.Fatalf("layouts[0].DPI = %d, want %d", l.DPI, wantDPI)
	}
	wantHeight := 400.0 / 72.0 * float64(wantDPI)
	if math.Abs(l.Height-wantHeight) > 0.01 {
		t.Fatalf("layouts[0].Height = %v, want %v", l.Height, wantHeight)
	}
	if math.Abs(total-l.Height) > 0.01 {
		t.Fatalf("total = %v, want %v (no trailing gap for a single page)", total, l.Height)
	}
}

func TestLayoutContinuous_MultiPageCumulativeOffsets(t *testing.T) {
	sizes := [][2]float64{{200, 400}, {200, 300}, {200, 500}}
	zoom := Zoom{Mode: ZoomFitWidth}
	const gap = 10.0
	layouts, total := LayoutContinuous(sizes, zoom, 200, gap) // scale = 1.0, dpi = 72

	if len(layouts) != 3 {
		t.Fatalf("len(layouts) = %d, want 3", len(layouts))
	}
	if layouts[0].Top != 0 {
		t.Fatalf("layouts[0].Top = %v, want 0", layouts[0].Top)
	}
	wantTop1 := layouts[0].Height + gap
	if math.Abs(layouts[1].Top-wantTop1) > 0.01 {
		t.Fatalf("layouts[1].Top = %v, want %v", layouts[1].Top, wantTop1)
	}
	wantTop2 := wantTop1 + layouts[1].Height + gap
	if math.Abs(layouts[2].Top-wantTop2) > 0.01 {
		t.Fatalf("layouts[2].Top = %v, want %v", layouts[2].Top, wantTop2)
	}
	wantTotal := layouts[2].Top + layouts[2].Height
	if math.Abs(total-wantTotal) > 0.01 {
		t.Fatalf("total = %v, want %v", total, wantTotal)
	}
}

func TestLayoutContinuous_ZoomChangeScalesTotalHeight(t *testing.T) {
	sizes := [][2]float64{{200, 400}, {200, 400}}
	small := Zoom{Mode: ZoomPercent, Percent: 50}
	big := Zoom{Mode: ZoomPercent, Percent: 200}

	_, totalSmall := LayoutContinuous(sizes, small, 800, 8)
	_, totalBig := LayoutContinuous(sizes, big, 800, 8)

	if totalBig <= totalSmall*3 { // 200% vs 50% is 4x the linear scale
		t.Fatalf("totalBig = %v, totalSmall = %v, want totalBig much larger", totalBig, totalSmall)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -run TestLayoutContinuous -v
```

Expected: FAIL，`LayoutContinuous`/`PageLayout` 未定义。

- [ ] **Step 3: 实现 continuous.go**

```go
// internal/document/continuous.go
package document

// PageLayout describes one page's position within the full scrollable
// content of a continuously-scrolled document, at a given zoom level.
type PageLayout struct {
	Top    float64 // px, top offset within the full virtual content
	Width  float64 // px
	Height float64 // px
	DPI    int     // dpi to render this page at (matches pdfengine.RenderPage's dpi param)
}

// LayoutContinuous computes per-page layout for continuous-scroll mode.
// pageSizesPt is each page's (widthPt, heightPt) in document order. zoom
// must not be ZoomFitPage - callers are responsible for downgrading to
// ZoomFitWidth first, since fit-page has no meaning when many pages are
// visible at once (passing 0 as the viewport height below makes a
// ZoomFitPage caller's mistake produce an obviously-wrong degenerate
// layout instead of a plausible-looking wrong one). viewportWidthPx is
// used for ZoomFitWidth/ZoomPercent scale calculation. gapPx is the
// vertical spacing drawn between consecutive pages.
func LayoutContinuous(pageSizesPt [][2]float64, zoom Zoom, viewportWidthPx, gapPx float64) ([]PageLayout, float64) {
	layouts := make([]PageLayout, len(pageSizesPt))
	top := 0.0
	for i, size := range pageSizesPt {
		widthPt, heightPt := size[0], size[1]
		scale := zoom.ScaleFactor(widthPt, heightPt, viewportWidthPx, 0)
		dpi := DPIForScale(scale)
		widthPx := widthPt / 72.0 * float64(dpi)
		heightPx := heightPt / 72.0 * float64(dpi)

		layouts[i] = PageLayout{Top: top, Width: widthPx, Height: heightPx, DPI: dpi}
		top += heightPx + gapPx
	}

	total := top
	if len(layouts) > 0 {
		total -= gapPx // no trailing gap after the last page
	}
	return layouts, total
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -run TestLayoutContinuous -v
```

Expected: 3 个测试全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/document/continuous.go internal/document/continuous_test.go
git commit -m "feat: add continuous-scroll page layout calculation"
```

---

### Task 2: document.VisiblePages / MostVisiblePage

**Files:**
- Modify: `internal/document/continuous.go`
- Test: `internal/document/continuous_test.go`

- [ ] **Step 1: 追加失败的测试**

在 `internal/document/continuous_test.go` 末尾追加：

```go
func TestVisiblePages_ViewportSpanningMultiplePages(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 108, Height: 100}, // 8px gap
		{Top: 216, Height: 100},
		{Top: 324, Height: 100},
	}

	start, end := VisiblePages(layouts, 150, 120) // viewport [150,270): touches pages 1,2

	if start != 1 || end != 3 {
		t.Fatalf("VisiblePages = [%d,%d), want [1,3)", start, end)
	}
}

func TestVisiblePages_GapOnlyViewportIsEmpty(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 108, Height: 100},
	}

	// viewport [100,108) sits entirely in the gap between the two pages.
	start, end := VisiblePages(layouts, 100, 8)

	if start != end {
		t.Fatalf("VisiblePages = [%d,%d), want an empty range in the gap", start, end)
	}
}

func TestVisiblePages_EmptyLayouts(t *testing.T) {
	start, end := VisiblePages(nil, 0, 100)
	if start != 0 || end != 0 {
		t.Fatalf("VisiblePages(nil) = [%d,%d), want [0,0)", start, end)
	}
}

func TestMostVisiblePage_PicksLargerOverlap(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 108, Height: 100},
	}

	// viewport [80,188): page 0 shows 20px (80..100), page 1 shows 80px (108..188).
	got := MostVisiblePage(layouts, 80, 108)

	if got != 1 {
		t.Fatalf("MostVisiblePage = %d, want 1", got)
	}
}

func TestMostVisiblePage_TieBreaksToEarlierPage(t *testing.T) {
	layouts := []PageLayout{
		{Top: 0, Height: 100},
		{Top: 100, Height: 100},
	}

	// viewport [50,150): both pages show exactly 50px.
	got := MostVisiblePage(layouts, 50, 100)

	if got != 0 {
		t.Fatalf("MostVisiblePage = %d, want 0 (tie -> earlier page)", got)
	}
}

func TestMostVisiblePage_EmptyLayouts(t *testing.T) {
	if got := MostVisiblePage(nil, 0, 100); got != 0 {
		t.Fatalf("MostVisiblePage(nil) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -run "TestVisiblePages|TestMostVisiblePage" -v
```

Expected: FAIL，`VisiblePages`/`MostVisiblePage` 未定义。

- [ ] **Step 3: 在 continuous.go 中实现**

在 `internal/document/continuous.go` 末尾追加：

```go
// VisiblePages returns the [start,end) index range (into layouts) of
// pages that intersect the vertical range [scrollTop, scrollTop+viewportHeight).
// Pages that only touch the range at an edge (no interior overlap) don't
// count. Returns (0, 0) if layouts is empty or nothing intersects.
func VisiblePages(layouts []PageLayout, scrollTop, viewportHeight float64) (start, end int) {
	scrollBottom := scrollTop + viewportHeight
	start = -1
	for i, l := range layouts {
		pageBottom := l.Top + l.Height
		if pageBottom <= scrollTop {
			continue
		}
		if l.Top >= scrollBottom {
			break
		}
		if start == -1 {
			start = i
		}
		end = i + 1
	}
	if start == -1 {
		return 0, 0
	}
	return start, end
}

// MostVisiblePage returns the index into layouts of the page covering
// the largest visible area within [scrollTop, scrollTop+viewportHeight).
// Returns 0 if layouts is empty. Ties resolve to the earlier (smaller
// index) page.
func MostVisiblePage(layouts []PageLayout, scrollTop, viewportHeight float64) int {
	if len(layouts) == 0 {
		return 0
	}
	scrollBottom := scrollTop + viewportHeight
	best := 0
	bestVisible := -1.0
	for i, l := range layouts {
		pageBottom := l.Top + l.Height
		visibleTop := l.Top
		if scrollTop > visibleTop {
			visibleTop = scrollTop
		}
		visibleBottom := pageBottom
		if scrollBottom < visibleBottom {
			visibleBottom = scrollBottom
		}
		visible := visibleBottom - visibleTop
		if visible > bestVisible {
			bestVisible = visible
			best = i
		}
	}
	return best
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -v
```

Expected: `internal/document` 下全部测试 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/document/continuous.go internal/document/continuous_test.go
git commit -m "feat: add VisiblePages/MostVisiblePage for continuous-scroll viewport queries"
```

---

### Task 3: config.ContinuousMode 持久化字段

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 追加失败的测试**

在 `internal/config/config_test.go` 末尾追加：

```go
func TestSaveThenLoad_ContinuousMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{ContinuousMode: true}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !loaded.ContinuousMode {
		t.Fatalf("loaded.ContinuousMode = false, want true")
	}
}

func TestLoad_MissingFileDefaultsToSinglePageMode(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFrom(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.ContinuousMode {
		t.Fatalf("cfg.ContinuousMode = true, want false (default is single-page mode)")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/config/... -run "ContinuousMode" -v
```

Expected: FAIL，`Config.ContinuousMode` 字段不存在（编译错误）。

- [ ] **Step 3: 在 config.go 中添加字段**

在 `internal/config/config.go` 的 `Config` 结构体里追加一个字段（`SidebarTab` 那行下面）：

```go
	SidebarTab   string       `json:"sidebarTab"` // "outline" or "thumbnails"
	ContinuousMode bool       `json:"continuousMode"`
```

（`defaultConfig()` 不需要改——零值 `false` 就是想要的默认单页模式。）

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/config/... -v
```

Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: persist continuous-reading-mode toggle in config"
```

---

### Task 4: outlineModel.findByPage — 按页码反查目录节点

**Files:**
- Modify: `internal/ui/outlinesidebar.go`
- Test: `internal/ui/outlinesidebar_test.go`

- [ ] **Step 1: 追加失败的测试**

在 `internal/ui/outlinesidebar_test.go` 末尾追加：

```go
func TestOutlineModel_FindByPage(t *testing.T) {
	nodes := []pdfengine.OutlineNode{
		{
			Title:     "Chapter 1",
			PageIndex: 0,
			Children: []pdfengine.OutlineNode{
				{Title: "Section 1.1", PageIndex: 1},
				{Title: "Section 1.2", PageIndex: 2},
			},
		},
		{Title: "Chapter 2", PageIndex: 3},
	}
	m := newOutlineModel(nodes)

	item := m.findByPage(2)
	if item == nil || item.Text() != "Section 1.2" {
		t.Fatalf("findByPage(2) = %v, want Section 1.2", item)
	}

	item = m.findByPage(3)
	if item == nil || item.Text() != "Chapter 2" {
		t.Fatalf("findByPage(3) = %v, want Chapter 2", item)
	}

	if item := m.findByPage(99); item != nil {
		t.Fatalf("findByPage(99) = %v, want nil", item)
	}
}

func TestOutlineModel_FindByPageEmpty(t *testing.T) {
	m := newOutlineModel(nil)
	if item := m.findByPage(0); item != nil {
		t.Fatalf("findByPage(0) on empty model = %v, want nil", item)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/ui/... -run FindByPage -v
```

Expected: FAIL，`findByPage` 未定义（编译错误）。

- [ ] **Step 3: 在 outlinesidebar.go 中实现**

在 `internal/ui/outlinesidebar.go` 末尾追加：

```go
// findByPage returns the first outline item (depth-first, root order)
// whose PageIndex equals page, or nil if none match. Used to sync the
// outline tree's selection to whichever page is currently most visible
// while scrolling in continuous reading mode.
func (m *outlineModel) findByPage(page int) *outlineItem {
	for _, root := range m.roots {
		if item := root.findByPage(page); item != nil {
			return item
		}
	}
	return nil
}

func (i *outlineItem) findByPage(page int) *outlineItem {
	if i.node.PageIndex == page {
		return i
	}
	for _, child := range i.children {
		if item := child.findByPage(page); item != nil {
			return item
		}
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/ui/... -run "TestOutlineModel" -v
```

Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/ui/outlinesidebar.go internal/ui/outlinesidebar_test.go
git commit -m "feat: add outlineModel.findByPage for scroll-driven outline sync"
```

---

### Task 5: tab 结构体扩展 + 共用渲染/滚动辅助函数

**Files:**
- Modify: `internal/ui/tab.go`
- Modify: `internal/ui/pageview.go`

- [ ] **Step 1: 扩展 tab 结构体**

编辑 `internal/ui/tab.go`，在 `tab` 结构体里追加字段（`pageView`/`outlineTree` 那两行附近），并把 `newTab` 的 cache 容量从 5 调到 16：

```go
// internal/ui/tab.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/document"
	"pdfreader/internal/pdfengine"
)

// tab holds all runtime state for one open document.
type tab struct {
	path string
	doc  *pdfengine.Document

	page  int // 0-based current page index
	zoom  document.Zoom
	cache *document.Cache

	outline     []pdfengine.OutlineNode
	outlineModel *outlineModel

	tabPage     *walk.TabPage
	pageScroll  *walk.ScrollView
	pageView    *walk.CustomWidget
	outlineTree *walk.TreeView

	// continuousLayout/continuousLayoutW/continuousTotalH cache the last
	// computed continuous-mode page layout (see ensureContinuousLayout in
	// pageview.go). continuousLayout is nil whenever it needs
	// recomputing (mode just turned on, zoom changed, or never computed).
	continuousLayout  []document.PageLayout
	continuousLayoutW float64
	continuousTotalH  float64

	searchMatches []pdfengine.SearchMatch
	searchIndex   int // index into searchMatches of the currently-highlighted match

	searchBar  *walk.Composite
	searchEdit *walk.LineEdit
}

func newTab(path string, doc *pdfengine.Document) *tab {
	return &tab{
		path:  path,
		doc:   doc,
		page:  0,
		zoom:  document.Zoom{Mode: document.ZoomFitPage},
		cache: document.NewCache(16),
	}
}
```

- [ ] **Step 2: 拆出共用渲染 helper，加连续模式的滚动读写 helper**

编辑 `internal/ui/pageview.go`，替换整个文件内容：

```go
// internal/ui/pageview.go
package ui

import (
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"

	"pdfreader/internal/document"
	"pdfreader/internal/pdfengine"
)

const continuousPageGapPx = 8

// renderPageBitmap renders page at dpi (using cache when possible) and
// returns a walk.Bitmap ready to paint. The caller owns the returned
// bitmap and must Dispose() it when done.
func renderPageBitmap(doc *pdfengine.Document, cache *document.Cache, page, dpi int) (*walk.Bitmap, error) {
	key := document.CacheKey{Page: page, DPI: dpi}
	img, ok := cache.Get(key)
	if !ok {
		rendered, err := doc.RenderPage(page, dpi)
		if err != nil {
			return nil, err
		}
		img = rendered
		cache.Put(key, img)
	}
	return walk.NewBitmapFromImage(img)
}

// renderCurrentPage renders t's current page at t's current zoom (using
// the cache when possible) and returns a walk.Bitmap ready to paint.
// The caller owns the returned bitmap and must Dispose() it when replaced.
// Only used in single-page mode - continuous mode renders each visible
// page directly via renderPageBitmap (see paintContinuousTab in app.go).
func (t *tab) renderCurrentPage(viewportW, viewportH float64) (*walk.Bitmap, error) {
	pageWidthPt, pageHeightPt, err := t.doc.PageSize(t.page)
	if err != nil {
		return nil, err
	}

	scale := t.zoom.ScaleFactor(pageWidthPt, pageHeightPt, viewportW, viewportH)
	dpi := document.DPIForScale(scale)

	return renderPageBitmap(t.doc, t.cache, t.page, dpi)
}

// ensureContinuousLayout recomputes t's continuous-mode page layout if it
// hasn't been computed yet (t.continuousLayout == nil) or the viewport
// width has changed since the last computation (which changes
// ZoomFitWidth/ZoomPercent scale-to-pixel results), and resizes pageView
// to match the new total virtual content size so pageScroll's scrollbar
// range stays correct.
func ensureContinuousLayout(t *tab, viewportWidthPx float64) error {
	if t.continuousLayout != nil && t.continuousLayoutW == viewportWidthPx {
		return nil
	}

	sizes := make([][2]float64, t.doc.PageCount())
	for i := range sizes {
		w, h, err := t.doc.PageSize(i)
		if err != nil {
			return err
		}
		sizes[i] = [2]float64{w, h}
	}

	layouts, total := document.LayoutContinuous(sizes, t.zoom, viewportWidthPx, continuousPageGapPx)

	maxWidth := viewportWidthPx
	for _, l := range layouts {
		if l.Width > maxWidth {
			maxWidth = l.Width
		}
	}

	t.continuousLayout = layouts
	t.continuousLayoutW = viewportWidthPx
	t.continuousTotalH = total

	return t.pageView.SetSizePixels(walk.Size{Width: int(maxWidth), Height: int(total)})
}

// continuousScrollY returns how far pageView has been scrolled down
// within its ScrollView, in native pixels.
//
// walk.ScrollView (see its scrollview.go) implements scrolling by moving
// an internal, unexported content composite up/down via SetYPixels; the
// widgets added to the ScrollView (here, pageView) become native
// children of that composite (walk re-parents them there - see
// ContainerBase.onInsertedWidget in container.go). So pageView.Parent()
// returns that composite as a walk.Container, and the composite's own Y
// position (relative to its parent, the fixed-size ScrollView viewport)
// is always -scrollY. A local interface is enough to reach the public
// BoundsPixels method without needing the unexported concrete type.
func continuousScrollY(pageView *walk.CustomWidget) float64 {
	parent, ok := pageView.Parent().(interface{ BoundsPixels() walk.Rectangle })
	if !ok {
		return 0
	}
	return -float64(parent.BoundsPixels().Y)
}

// setContinuousScrollY scrolls pageScroll so pageView's content shows y
// (clamped to the valid range implied by totalHeight) at the top of the
// viewport, and keeps pageScroll's native scrollbar thumb in sync.
//
// walk.ScrollView has no public "scroll to" method - scroll() and
// updateScrollBars() in its scrollview.go are both unexported. Moving
// the composite directly via SetYPixels (see continuousScrollY above)
// would move the content but leave the native scrollbar thumb pointing
// at the old position, since only ScrollView's own WM_VSCROLL handler
// updates the win32 scrollbar's position. So this reaches into win32
// directly and does what that handler would do for a user-driven
// scroll: set the scrollbar's logical position, then move the content
// to match.
func setContinuousScrollY(pageScroll *walk.ScrollView, pageView *walk.CustomWidget, y, totalHeight float64) {
	viewportH := float64(pageScroll.ClientBoundsPixels().Height)
	maxY := totalHeight - viewportH
	if maxY < 0 {
		maxY = 0
	}
	if y < 0 {
		y = 0
	} else if y > maxY {
		y = maxY
	}

	parent, ok := pageView.Parent().(interface{ SetYPixels(int) error })
	if !ok {
		return
	}
	parent.SetYPixels(-int(y))

	si := win.SCROLLINFO{
		CbSize: uint32(unsafe.Sizeof(win.SCROLLINFO{})),
		FMask:  win.SIF_POS,
		NPos:   int32(y),
	}
	win.SetScrollInfo(pageScroll.Handle(), win.SB_VERT, &si, true)

	pageView.Invalidate()
}
```

- [ ] **Step 3: 编译确认没有语法错误**

```bash
cd "E:/workspace_go/PDFReader"
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build ./... 2>&1
```

Expected: 报错——`app.go` 里旧的 `paintTab`/`openFile` 还在用被删掉的 `renderCurrentPage` 之外的代码路径，且 `t.pageScroll`/`applyPageViewMode` 还没接上。这一步只是确认新增的这两个文件本身语法正确、`internal/document`/`internal/pdfengine` 的引用没写错，`internal/ui` 包级别的编译错误留到 Task 6-8 逐步修完。用下面这条更精确地只检查语法（不link）：

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" vet ./internal/document/... ./internal/config/...
```

Expected: 无输出（这两个包已经完整、能独立编译通过）。

- [ ] **Step 4: 提交**

```bash
git add internal/ui/tab.go internal/ui/pageview.go
git commit -m "feat: add tab fields and shared render/scroll helpers for continuous mode"
```

（这一步提交的代码还没被 `app.go` 接上，属于预期中的中间状态——`internal/ui` 整体编译会在 Task 6 之后才恢复绿色，这是本计划里唯一一次允许中间态不可编译的提交，因为 `tab.go`/`pageview.go` 本身语法和类型都是自洽的，只是还没被调用方接上。）

---

### Task 6: app.go — 控件树改造（splitter → pageScroll → pageView）

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 把 pageView 包进 pageScroll，更新拉伸比例**

在 `internal/ui/app.go` 的 `openFile` 函数里，找到这一段（Task 21 之前刚修过拉伸比例的地方）：

```go
	pageView, err := walk.NewCustomWidget(splitter, 0, func(canvas *walk.Canvas, updateBounds walk.Rectangle) error {
		return a.paintTab(t, canvas, updateBounds)
	})
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	pageView.SetClearsBackground(true)

	// Give the page view most of the splitter's width by default - an
	// outline/thumbnails sidebar only needs enough room to read titles or
	// see a thumbnail, not half the window. stretchFactor is unexported on
	// splitterLayout, but it satisfies this method set (same pattern
	// declarative/builder.go uses for its StretchFactor field), so a local
	// interface is enough to reach it without walk exporting the type.
	type stretchFactorSetter interface {
		SetStretchFactor(widget walk.Widget, factor int) error
	}
	if sfs, ok := splitter.Layout().(stretchFactorSetter); ok {
		sfs.SetStretchFactor(sidebarComposite, 1)
		sfs.SetStretchFactor(pageView, 4)
	}
```

替换成：

```go
	pageScroll, err := walk.NewScrollView(splitter)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	pageScroll.SetScrollbars(false, true) // no horizontal bar; the vertical one only does anything in continuous mode

	pageView, err := walk.NewCustomWidget(pageScroll, 0, func(canvas *walk.Canvas, updateBounds walk.Rectangle) error {
		return a.paintTab(t, canvas, updateBounds)
	})
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	pageView.SetClearsBackground(true)

	// Give the page view most of the splitter's width by default - an
	// outline/thumbnails sidebar only needs enough room to read titles or
	// see a thumbnail, not half the window. stretchFactor is unexported on
	// splitterLayout, but it satisfies this method set (same pattern
	// declarative/builder.go uses for its StretchFactor field), so a local
	// interface is enough to reach it without walk exporting the type.
	type stretchFactorSetter interface {
		SetStretchFactor(widget walk.Widget, factor int) error
	}
	if sfs, ok := splitter.Layout().(stretchFactorSetter); ok {
		sfs.SetStretchFactor(sidebarComposite, 1)
		sfs.SetStretchFactor(pageScroll, 4)
	}

	t.pageScroll = pageScroll
	pageScroll.SizeChanged().Attach(func() {
		a.applyPageViewMode(t)
	})
```

- [ ] **Step 2: 记录 outline model，并在 openFile 末尾套用当前阅读模式**

在 `openFile` 里，找到构建目录大纲的这几行：

```go
	treeView, err := walk.NewTreeView(outlinePage)
	if err != nil {
		outlinePage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}
	if err := treeView.SetModel(newOutlineModel(outline)); err != nil {
		outlinePage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}
```

改成保留一份 model 引用：

```go
	treeView, err := walk.NewTreeView(outlinePage)
	if err != nil {
		outlinePage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}
	outlineModel := newOutlineModel(outline)
	if err := treeView.SetModel(outlineModel); err != nil {
		outlinePage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}
```

再往下找到 `t.outlineTree = treeView`，改成：

```go
	t.outlineTree = treeView
	t.outlineModel = outlineModel
```

最后，在 `openFile` 函数末尾、`return nil` 之前（`a.rebuildRecentMenu()` 那行之后）追加一行，让新打开的标签页立刻套用当前的阅读模式：

```go
	a.applyPageViewMode(t)

	return nil
}
```

- [ ] **Step 3: 实现 applyPageViewMode**

在 `internal/ui/app.go` 里 `paintTab` 函数前面（或任意 `app` 方法附近）新增：

```go
// applyPageViewMode sizes t.pageView to match the app's current reading
// mode (single page vs continuous) and t's current zoom. It's the single
// place that decides pageView's size - called right after a tab is
// created (openFile), whenever its viewport resizes (pageScroll's
// SizeChanged, wired in openFile), whenever its zoom changes (setZoom),
// and whenever the continuous-mode toggle flips for every open tab
// (onToggleContinuousMode).
func (a *app) applyPageViewMode(t *tab) {
	if t.pageScroll == nil || t.pageView == nil {
		return
	}
	viewport := t.pageScroll.ClientBoundsPixels()
	if viewport.Width <= 0 || viewport.Height <= 0 {
		return
	}

	if !a.cfg.ContinuousMode {
		t.continuousLayout = nil
		t.pageView.SetSizePixels(walk.Size{Width: viewport.Width, Height: viewport.Height})
		t.pageView.Invalidate()
		return
	}

	if err := ensureContinuousLayout(t, float64(viewport.Width)); err != nil {
		return // leave whatever layout was computed last time in place
	}
	t.pageView.Invalidate()
}
```

- [ ] **Step 4: 编译**

```bash
cd "E:/workspace_go/PDFReader"
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build ./... 2>&1
```

Expected: 仍然会报错——`paintTab`/`goToPage`/`setZoom` 还没改，`a.cfg.ContinuousMode` 引用没问题但绘制逻辑还是旧的单页写法，行为上暂时不对（连续模式下也只会画当前一页，因为 `paintTab` 还没分支）。这一步只确认新增/改动的代码本身没有编译错误。如果这条命令本身报语法错误（不是"行为不对"而是"编译不过"），说明 Step 1-3 抄漏了什么，回去核对。

- [ ] **Step 5: 提交**

```bash
git add internal/ui/app.go
git commit -m "feat: wrap page view in a ScrollView, wire applyPageViewMode"
```

---

### Task 7: app.go — paintTab 按模式分支渲染

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 替换 paintTab，新增 paintContinuousTab / updateCurrentPageFromScroll**

找到现在的：

```go
func (a *app) paintTab(t *tab, canvas *walk.Canvas, updateBounds walk.Rectangle) error {
	bounds := t.pageView.ClientBounds()
	bmp, err := t.renderCurrentPage(float64(bounds.Width), float64(bounds.Height))
	if err != nil {
		return canvas.DrawText(err.Error(), nil, walk.RGB(200, 0, 0), updateBounds, walk.TextWordbreak)
	}
	defer bmp.Dispose()

	return canvas.DrawImage(bmp, walk.Point{X: 0, Y: 0})
}
```

替换成：

```go
func (a *app) paintTab(t *tab, canvas *walk.Canvas, updateBounds walk.Rectangle) error {
	if a.cfg.ContinuousMode {
		return a.paintContinuousTab(t, canvas)
	}

	bounds := t.pageView.ClientBounds()
	bmp, err := t.renderCurrentPage(float64(bounds.Width), float64(bounds.Height))
	if err != nil {
		return canvas.DrawText(err.Error(), nil, walk.RGB(200, 0, 0), updateBounds, walk.TextWordbreak)
	}
	defer bmp.Dispose()

	return canvas.DrawImage(bmp, walk.Point{X: 0, Y: 0})
}

// paintContinuousTab draws every page that intersects the current
// scroll viewport (see document.VisiblePages) at its computed offset,
// and updates t.page/the status bar/the outline selection to track
// whichever page is now most visible.
func (a *app) paintContinuousTab(t *tab, canvas *walk.Canvas) error {
	viewport := t.pageScroll.ClientBoundsPixels()
	if err := ensureContinuousLayout(t, float64(viewport.Width)); err != nil {
		return canvas.DrawText(err.Error(), nil, walk.RGB(200, 0, 0), viewport, walk.TextWordbreak)
	}

	scrollY := continuousScrollY(t.pageView)
	viewportH := float64(viewport.Height)

	start, end := document.VisiblePages(t.continuousLayout, scrollY, viewportH)
	for i := start; i < end; i++ {
		layout := t.continuousLayout[i]
		bmp, err := renderPageBitmap(t.doc, t.cache, i, layout.DPI)
		if err != nil {
			continue // skip a single bad page rather than failing the whole view
		}
		x := 0
		if layout.Width < float64(viewport.Width) {
			x = int((float64(viewport.Width) - layout.Width) / 2) // center narrower pages horizontally
		}
		y := int(layout.Top - scrollY)
		drawErr := canvas.DrawImagePixels(bmp, walk.Point{X: x, Y: y})
		bmp.Dispose()
		if drawErr != nil {
			return drawErr
		}
	}

	a.updateCurrentPageFromScroll(t, scrollY, viewportH)

	return nil
}

// updateCurrentPageFromScroll re-derives t.page from the current scroll
// position and, if it changed, refreshes the status bar and the outline
// tree's selection. This runs on every continuous-mode repaint (i.e. on
// every scroll step, since scrolling repaints pageView) rather than
// through a dedicated scroll event, because walk.ScrollView doesn't
// expose one publicly.
func (a *app) updateCurrentPageFromScroll(t *tab, scrollY, viewportH float64) {
	if len(t.continuousLayout) == 0 {
		return
	}
	page := document.MostVisiblePage(t.continuousLayout, scrollY, viewportH)
	if page == t.page {
		return
	}
	t.page = page
	a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", t.page+1, t.doc.PageCount()))
	if t.outlineModel != nil && t.outlineTree != nil {
		if item := t.outlineModel.findByPage(t.page); item != nil {
			t.outlineTree.SetCurrentItem(item)
		}
	}
}
```

- [ ] **Step 2: 编译**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build ./... 2>&1
```

Expected: 成功（`document` 包已经在 app.go 顶部被导入，`fmt` 也已经导入，不需要加新 import）。

- [ ] **Step 3: 提交**

```bash
git add internal/ui/app.go
git commit -m "feat: paint visible pages and track current page in continuous mode"
```

---

### Task 8: app.go — goToPage / setZoom 按模式分支

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: goToPage 加连续模式滚动分支**

找到：

```go
func (a *app) goToPage(t *tab, page int) {
	if t == nil {
		return
	}
	if page < 0 {
		page = 0
	}
	if last := t.doc.PageCount() - 1; page > last {
		page = last
	}
	t.page = page
	t.pageView.Invalidate()
	a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", t.page+1, t.doc.PageCount()))
}
```

替换成：

```go
func (a *app) goToPage(t *tab, page int) {
	if t == nil {
		return
	}
	if page < 0 {
		page = 0
	}
	if last := t.doc.PageCount() - 1; page > last {
		page = last
	}
	t.page = page

	if a.cfg.ContinuousMode {
		viewport := t.pageScroll.ClientBoundsPixels()
		if err := ensureContinuousLayout(t, float64(viewport.Width)); err == nil && page < len(t.continuousLayout) {
			setContinuousScrollY(t.pageScroll, t.pageView, t.continuousLayout[page].Top, t.continuousTotalH)
		}
	} else {
		t.pageView.Invalidate()
	}

	a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", t.page+1, t.doc.PageCount()))
}
```

- [ ] **Step 2: setZoom 加连续模式重新布局分支**

找到：

```go
func (a *app) setZoom(t *tab, z document.Zoom) {
	if t == nil {
		return
	}
	t.zoom = z
	t.pageView.Invalidate()
}
```

替换成：

```go
func (a *app) setZoom(t *tab, z document.Zoom) {
	if t == nil {
		return
	}
	if a.cfg.ContinuousMode && z.Mode == document.ZoomFitPage {
		z = document.Zoom{Mode: document.ZoomFitWidth} // fit-page is meaningless once many pages are visible at once
	}

	anchorPage := t.page
	t.zoom = z

	if a.cfg.ContinuousMode {
		viewport := t.pageScroll.ClientBoundsPixels()
		t.continuousLayout = nil // force a recompute at the new zoom
		if err := ensureContinuousLayout(t, float64(viewport.Width)); err == nil && anchorPage < len(t.continuousLayout) {
			// Simplification: scroll back to the top of the page that was
			// current before the zoom change, rather than preserving the
			// exact pixel offset within that page - keeping the fractional
			// scroll position pixel-perfect across a zoom change would need
			// converting it through the old and new scale factors for
			// marginal benefit.
			setContinuousScrollY(t.pageScroll, t.pageView, t.continuousLayout[anchorPage].Top, t.continuousTotalH)
		}
	}

	t.pageView.Invalidate()
}
```

- [ ] **Step 3: 编译**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build ./... 2>&1
```

Expected: 成功。

- [ ] **Step 4: 提交**

```bash
git add internal/ui/app.go
git commit -m "feat: scroll-to-page navigation and zoom-preserving re-layout in continuous mode"
```

---

### Task 9: app.go — "连续阅读模式"菜单项与"适合页面"禁用联动

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: app 结构体加两个 Action 引用字段**

找到 `app` 结构体：

```go
type app struct {
	pool *pdfengine.Pool
	cfg  *config.Config

	mainWindow *walk.MainWindow
	tabWidget  *walk.TabWidget
	statusBar  *walk.StatusBarItem

	recentMenuAction *walk.Action

	tabs []*tab
}
```

改成：

```go
type app struct {
	pool *pdfengine.Pool
	cfg  *config.Config

	mainWindow *walk.MainWindow
	tabWidget  *walk.TabWidget
	statusBar  *walk.StatusBarItem

	recentMenuAction     *walk.Action
	continuousModeAction *walk.Action
	fitPageAction        *walk.Action

	tabs []*tab
}
```

- [ ] **Step 2: 在"视图"菜单里加菜单项，给"适合页面"接上 AssignTo**

找到 `MenuItems` 里的"视图"菜单：

```go
			Menu{
				Text: "视图(&V)",
				Items: []MenuItem{
					Action{Text: "放大", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyOEMPlus}, OnTriggered: a.onZoomIn},
					Action{Text: "缩小", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyOEMMinus}, OnTriggered: a.onZoomOut},
					Action{Text: "适合宽度", OnTriggered: a.onFitWidth},
					Action{Text: "适合页面", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.Key0}, OnTriggered: a.onFitPage},
					Action{Text: "查找", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyF}, OnTriggered: a.onToggleSearch},
				},
			},
```

替换成：

```go
			Menu{
				Text: "视图(&V)",
				Items: []MenuItem{
					Action{Text: "放大", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyOEMPlus}, OnTriggered: a.onZoomIn},
					Action{Text: "缩小", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyOEMMinus}, OnTriggered: a.onZoomOut},
					Action{Text: "适合宽度", OnTriggered: a.onFitWidth},
					Action{Text: "适合页面", AssignTo: &a.fitPageAction, Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.Key0}, OnTriggered: a.onFitPage},
					Action{Text: "查找", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyF}, OnTriggered: a.onToggleSearch},
					Separator{},
					Action{Text: "连续阅读模式", AssignTo: &a.continuousModeAction, Checkable: true, Checked: a.cfg.ContinuousMode, OnTriggered: a.onToggleContinuousMode},
				},
			},
```

- [ ] **Step 3: 加 onToggleContinuousMode，并在窗口创建后应用初始的"适合页面"启用状态**

在 `Run` 函数里，找到：

```go
	if err := mw.Create(); err != nil {
		return 1, err
	}
	a.rebuildRecentMenu()
```

替换成：

```go
	if err := mw.Create(); err != nil {
		return 1, err
	}
	a.rebuildRecentMenu()
	a.fitPageAction.SetEnabled(!a.cfg.ContinuousMode)
```

然后在 `onToggleSearch` 函数附近新增：

```go
// onToggleContinuousMode flips the global continuous-reading-mode
// setting (all tabs share one mode), persists it, and re-applies it to
// every currently open tab.
func (a *app) onToggleContinuousMode() {
	a.cfg.ContinuousMode = a.continuousModeAction.Checked()
	a.cfg.Save()
	a.fitPageAction.SetEnabled(!a.cfg.ContinuousMode)

	for _, t := range a.tabs {
		if a.cfg.ContinuousMode && t.zoom.Mode == document.ZoomFitPage {
			t.zoom = document.Zoom{Mode: document.ZoomFitWidth}
		}
		a.applyPageViewMode(t)
	}
}
```

- [ ] **Step 4: 编译**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -ldflags "-H=windowsgui" -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 5: 提交**

```bash
git add internal/ui/app.go
git commit -m "feat: add continuous-reading-mode menu toggle"
```

---

### Task 10: 手动验证 + 更新 README 清单 + 收尾提交

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 跑一遍全部自动化测试**

```bash
cd "E:/workspace_go/PDFReader"
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./... -v
```

Expected: 全部 PASS（`internal/document`、`internal/config`、`internal/pdfengine`、`internal/ui`）。

- [ ] **Step 2: 构建并手动验证**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -ldflags "-H=windowsgui" -o pdfreader.exe .
./pdfreader.exe testdata/sample.pdf
```

逐项验证（这部分和之前所有 UI 任务一样没有自动化测试，需要人工在实际运行的 `pdfreader.exe` 上确认）：

1. 打开"视图"菜单，勾选"连续阅读模式"——页面变成可以纵向滚动，"适合页面"菜单项变灰不可点。
2. 滚动鼠标滚轮，页面依次渲染，没有明显卡顿或白屏；把窗口缩小再放大，滚动仍然正常。
3. 滚动过程中观察状态栏"第 X / Y 页"是否随视口内主要可见的页面更新；如果当前文档有目录（用 `testdata/sample.pdf`：书签"Page One"/"Page Two"），侧边栏目录的高亮选中是否跟着滚动切换。
4. 点击目录书签、点击缩略图、Ctrl+G 跳转页码、查找的上一个/下一个匹配——确认都能正确滚动到目标页顶部。
5. 放大/缩小/适合宽度——确认缩放后滚动位置视觉上还停留在缩放前那一页附近（不会跳到文档开头或结尾）。
6. 取消勾选"连续阅读模式"，回到单页模式——确认翻页、缩放、查找等和这次改动之前完全一样。
7. 关闭程序，重新用 `./pdfreader.exe testdata/sample.pdf` 打开——确认连续阅读模式的勾选状态被记住了。

如果第 3 步的目录高亮跟随滚动没有实现缩略图侧边栏的同步高亮：这是计划阶段就明确的范围裁剪——现有 `thumbnails.go` 里的缩略图完全没有"选中态"这个视觉概念（不像目录树有 `SetCurrentItem` 可以直接复用），要做缩略图高亮需要先给每个缩略图加一个边框/背景色的选中态，属于新的子功能，这次不做，只做了目录树高亮跟随。

- [ ] **Step 3: 更新 README 手动测试清单**

在 `README.md` 的"手动测试清单"末尾追加：

```markdown
- [ ] 连续阅读模式：勾选后可纵向滚动全文档，滚动流畅无明显卡顿/白屏
- [ ] 连续阅读模式：状态栏页码和目录侧边栏高亮跟随视口内主要可见页更新
- [ ] 连续阅读模式：点击目录/缩略图/跳转页码/查找匹配，正确滚动到目标页顶部
- [ ] 连续阅读模式：缩放（放大/缩小/适合宽度）后滚动位置视觉上保持在同一页附近；"适合页面"菜单项禁用
- [ ] 连续阅读模式：取消勾选后单页模式行为和之前完全一致
- [ ] 连续阅读模式：关闭程序重新打开，勾选状态被记住
```

同时在"功能"小节的列表里加一条：

```markdown
- 连续阅读模式（可切换，虚拟滚动，仅渲染可视页）
```

- [ ] **Step 4: 提交**

```bash
git add README.md
git commit -m "docs: add continuous reading mode to feature list and manual test checklist"
```
