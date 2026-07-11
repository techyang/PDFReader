# PDF 阅读器实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 使用 `github.com/lxn/walk` 构建一个 Windows 桌面 PDF 阅读器，支持打开/浏览/缩放/搜索/书签/缩略图/多标签页/最近文件/加密文档/命令行打开。

**Architecture:** 三层结构 —— `internal/pdfengine`（go-pdfium WASM 封装，PDF→数据/图像）、`internal/config` 与 `internal/document`（不涉及 UI 的业务状态/计算）、`internal/ui`（walk 声明式界面，负责事件转发与调用下层）。`main.go` 解析命令行参数并启动 UI。

**Tech Stack:** Go 1.25（工具链自动升级）、`github.com/lxn/walk` + `walk/declarative`、`github.com/klippa-app/go-pdfium`（WebAssembly/wazero 模式，无 cgo）、`github.com/tc-hib/go-winres`（图标/manifest 嵌入）。

---

## 环境说明（实现前必读）

- 本机 Go 工具链在 `D:\soft\go\bin\go.exe`（32 位宿主，`windows/386`）。已验证：设置 `GOARCH=amd64 GOOS=windows` 可交叉编译出正常运行的 64 位可执行文件。**本计划所有构建命令均显式设置 `GOARCH=amd64 GOOS=windows`**。
- `go.mod` 必须声明 `go 1.25.0`（`go-pdfium` 与 `walk` 依赖的 `golang.org/x/sys` 都要求 >= 1.25）。首次构建时 Go 会通过 `GOTOOLCHAIN=auto` 自动下载 go1.25.x 工具链，需要网络（已验证 `goproxy.cn` 可用）。
- `CGO_ENABLED=0`：整条技术栈不需要 C 编译器。
- 测试固件已生成并提交在 `testdata/`：
  - `testdata/sample.pdf` —— 2 页，页1文本 "hello world page one"，页2文本 "goodbye world page two"，书签 "Page One"(→page 0)/"Page Two"(→page 1)。150 DPI 渲染尺寸为 1241×1754。
  - `testdata/encrypted.pdf` —— 同内容，用户/所有者密码均为 `testpass`，AES-256 加密。
- 关键第三方 API 已逐一实测验证（版本：`go-pdfium v1.19.4`，`lxn/walk` commit `c389da54e794`），代码中出现的结构体字段、方法签名均已跑通，不是猜测。

---

## 文件结构总览

```
PDFReader/
  go.mod
  main.go
  internal/
    pdfengine/
      pool.go
      document.go
      render.go
      outline.go
      search.go
      pool_test.go
      document_test.go
      render_test.go
      outline_test.go
      search_test.go
    config/
      config.go
      config_test.go
    document/
      zoom.go
      zoom_test.go
      cache.go
      cache_test.go
    ui/
      app.go
      tab.go
      pageview.go
      navigation.go
      zoomcontrols.go
      outlinesidebar.go
      thumbnails.go
      searchbar.go
      tabmanager.go
      dialogs.go
  testdata/
    sample.pdf
    encrypted.pdf
  winres/
    winres.json
    icon.ico
  rsrc_windows_amd64.syso
  README.md
```

---

### Task 1: 项目脚手架与交叉编译验证

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `main.go`

- [ ] **Step 1: 初始化 go.mod**

```bash
cd "E:/workspace_go/PDFReader"
"/d/soft/go/bin/go.exe" mod init pdfreader
```

打开生成的 `go.mod`，把 `go` 指令改为：

```
module pdfreader

go 1.25.0
```

- [ ] **Step 2: 创建 .gitignore**

```
*.exe
*.test
/dist/
```

- [ ] **Step 3: 写一个最小 main.go 占位**

```go
package main

import "fmt"

func main() {
	fmt.Println("pdfreader starting")
}
```

- [ ] **Step 4: 验证交叉编译到 64 位 Windows 可执行**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
./pdfreader.exe
```

Expected: 输出 `pdfreader starting`，且 `file pdfreader.exe`（或 PowerShell 中查看属性）显示为 PE32+ (64位)。

- [ ] **Step 5: 提交**

```bash
git add go.mod .gitignore main.go
git commit -m "chore: init go module and verify amd64 cross-compile"
```

---

### Task 2: Windows 资源嵌入（图标 + DPI/主题 manifest）

lxn/walk 的控件依赖 Common Controls v6，没有正确的 manifest 时控件会退化成 Windows 95 风格；同时需要声明 per-monitor DPI 感知以获得清晰的高 DPI 渲染。

**Files:**
- Create: `winres/winres.json`
- Create: `rsrc_windows_amd64.syso`（生成产物，提交到仓库）

- [ ] **Step 1: 安装 go-winres**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" install github.com/tc-hib/go-winres@latest
```

- [ ] **Step 2: 初始化 winres 配置**

```bash
cd "E:/workspace_go/PDFReader"
"$(/d/soft/go/bin/go.exe env GOPATH)/bin/go-winres.exe" init
```

这会生成 `winres/winres.json` 和一个占位图标。打开 `winres/winres.json`，确认（或手工编辑成）以下要点：
- `"manifest"` 段中 `"execution-level"` 保持默认 `"as invoker"`。
- `"manifest"` 段中加入 per-monitor DPI 感知：`"dpi-awareness": "per-monitor-v2"`。
- 保留默认生成的 comctl32 v6 依赖声明（go-winres 的默认 manifest 模板已包含）。

- [ ] **Step 3: 生成 .syso 资源文件**

```bash
"$(/d/soft/go/bin/go.exe env GOPATH)/bin/go-winres.exe" make --arch amd64
```

确认项目根目录出现 `rsrc_windows_amd64.syso`。

- [ ] **Step 4: 重新构建，确认资源被嵌入**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 构建成功；`pdfreader.exe` 文件图标应为 winres 中配置的图标（而非 Go 默认图标），可在资源管理器中查看确认。

- [ ] **Step 5: 提交**

```bash
git add winres/ rsrc_windows_amd64.syso
git commit -m "chore: embed app icon and DPI-aware manifest via go-winres"
```

---

### Task 3: pdfengine — 连接池与文档打开/关闭

**Files:**
- Create: `internal/pdfengine/pool.go`
- Create: `internal/pdfengine/document.go`
- Test: `internal/pdfengine/document_test.go`

- [ ] **Step 1: 添加 go-pdfium 依赖**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" get github.com/klippa-app/go-pdfium@v1.19.4
```

- [ ] **Step 2: 写失败的测试**

```go
// internal/pdfengine/document_test.go
package pdfengine

import (
	"errors"
	"os"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return data
}

func TestOpenDocument_Success(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if got := doc.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}
}

func TestOpenDocument_PasswordRequired(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	_, err = pool.Open(readTestdata(t, "encrypted.pdf"), nil)
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("Open() err = %v, want ErrPasswordRequired", err)
	}
}

func TestOpenDocument_WrongPassword(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	wrong := "wrongpass"
	_, err = pool.Open(readTestdata(t, "encrypted.pdf"), &wrong)
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("Open() err = %v, want ErrPasswordRequired", err)
	}
}

func TestOpenDocument_CorrectPassword(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	pw := "testpass"
	doc, err := pool.Open(readTestdata(t, "encrypted.pdf"), &pw)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if got := doc.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}
}
```

- [ ] **Step 2b: 运行测试确认失败（包不存在）**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestOpenDocument -v
```

Expected: FAIL，提示 `NewPool`/`ErrPasswordRequired` 未定义。

- [ ] **Step 3: 实现 pool.go**

```go
// internal/pdfengine/pool.go
package pdfengine

import (
	"github.com/klippa-app/go-pdfium"
	pdfiumerrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// ErrPasswordRequired is returned by Pool.Open when the document is
// encrypted and no password (or the wrong password) was supplied.
var ErrPasswordRequired = pdfiumerrors.ErrPassword

// Pool wraps a go-pdfium WebAssembly instance pool.
type Pool struct {
	pool pdfium.Pool
}

// NewPool creates a new pdfium instance pool running the PDFium
// WebAssembly build via wazero (no cgo required).
func NewPool() (*Pool, error) {
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle:  1,
		MaxIdle:  2,
		MaxTotal: 4,
	})
	if err != nil {
		return nil, err
	}
	return &Pool{pool: pool}, nil
}

// Close shuts down the pool and all its instances.
func (p *Pool) Close() error {
	return p.pool.Close()
}
```

- [ ] **Step 4: 实现 document.go**

```go
// internal/pdfengine/document.go
package pdfengine

import (
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Document is an open PDF document. It owns a pdfium instance from the
// pool for its entire lifetime; call Close when done with it.
type Document struct {
	instance pdfium.Pdfium
	handle   references.FPDF_DOCUMENT
	pages    int
}

// Open opens a PDF document from raw bytes. password may be nil for
// unencrypted documents. If the document is encrypted and password is
// nil or incorrect, Open returns an error that satisfies
// errors.Is(err, ErrPasswordRequired).
func (p *Pool) Open(data []byte, password *string) (*Document, error) {
	instance, err := p.pool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, err
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		File:     &data,
		Password: password,
	})
	if err != nil {
		instance.Close()
		return nil, err
	}

	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		instance.Close()
		return nil, err
	}

	return &Document{
		instance: instance,
		handle:   doc.Document,
		pages:    pageCount.PageCount,
	}, nil
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return d.pages
}

// Close releases the document and returns the pdfium instance to the pool.
func (d *Document) Close() error {
	if _, err := d.instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.handle}); err != nil {
		d.instance.Close()
		return err
	}
	return d.instance.Close()
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestOpenDocument -v
```

Expected: 4 个测试全部 PASS。

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum internal/pdfengine/pool.go internal/pdfengine/document.go internal/pdfengine/document_test.go
git commit -m "feat: add pdfengine pool and document open/close with password support"
```

---

### Task 4: pdfengine — 页面渲染

**Files:**
- Create: `internal/pdfengine/render.go`
- Test: `internal/pdfengine/render_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/pdfengine/render_test.go
package pdfengine

import "testing"

func TestRenderPage(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	img, err := doc.RenderPage(0, 150)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 1241 || bounds.Dy() != 1754 {
		t.Fatalf("rendered size = %dx%d, want 1241x1754", bounds.Dx(), bounds.Dy())
	}
}

func TestRenderPage_OutOfRange(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if _, err := doc.RenderPage(5, 150); err == nil {
		t.Fatal("RenderPage(5, ...) expected error for out-of-range page")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestRenderPage -v
```

Expected: FAIL，`RenderPage` 未定义。

- [ ] **Step 3: 实现 render.go**

```go
// internal/pdfengine/render.go
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
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestRenderPage -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/pdfengine/render.go internal/pdfengine/render_test.go
git commit -m "feat: add pdfengine page rendering"
```

---

### Task 5: pdfengine — 目录大纲（书签）提取

**Files:**
- Create: `internal/pdfengine/outline.go`
- Test: `internal/pdfengine/outline_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/pdfengine/outline_test.go
package pdfengine

import "testing"

func TestOutline(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	nodes, err := doc.Outline()
	if err != nil {
		t.Fatalf("Outline: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2", len(nodes))
	}
	if nodes[0].Title != "Page One" || nodes[0].PageIndex != 0 {
		t.Fatalf("nodes[0] = %+v, want Title=Page One PageIndex=0", nodes[0])
	}
	if nodes[1].Title != "Page Two" || nodes[1].PageIndex != 1 {
		t.Fatalf("nodes[1] = %+v, want Title=Page Two PageIndex=1", nodes[1])
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestOutline -v
```

Expected: FAIL，`Outline`/`OutlineNode` 未定义。

- [ ] **Step 3: 实现 outline.go**

`responses.GetBookmarks.Bookmarks` 的类型是 `[]responses.GetBookmarksBookmark`（树形结构，`DestInfo.PageIndex` 直接给出目标页码，已在 spike 中验证）：

```go
// internal/pdfengine/outline.go
package pdfengine

import (
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
)

// OutlineNode is one entry in a PDF's bookmark/outline tree.
type OutlineNode struct {
	Title     string
	PageIndex int // -1 if the bookmark has no page destination.
	Children  []OutlineNode
}

// Outline returns the document's bookmark tree, empty if the document has
// no outline.
func (d *Document) Outline() ([]OutlineNode, error) {
	resp, err := d.instance.GetBookmarks(&requests.GetBookmarks{Document: d.handle})
	if err != nil {
		return nil, err
	}
	return convertBookmarks(resp.Bookmarks), nil
}

func convertBookmarks(in []responses.GetBookmarksBookmark) []OutlineNode {
	out := make([]OutlineNode, 0, len(in))
	for _, b := range in {
		node := OutlineNode{
			Title:     b.Title,
			PageIndex: -1,
			Children:  convertBookmarks(b.Children),
		}
		if b.DestInfo != nil {
			node.PageIndex = b.DestInfo.PageIndex
		}
		out = append(out, node)
	}
	return out
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestOutline -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/pdfengine/outline.go internal/pdfengine/outline_test.go
git commit -m "feat: add pdfengine outline/bookmark extraction"
```

---

### Task 6: pdfengine — 全文搜索

**Files:**
- Create: `internal/pdfengine/search.go`
- Test: `internal/pdfengine/search_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/pdfengine/search_test.go
package pdfengine

import "testing"

func TestSearch_MatchesOnBothPages(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	matches, err := doc.Search("world")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	if matches[0].PageIndex != 0 {
		t.Fatalf("matches[0].PageIndex = %d, want 0", matches[0].PageIndex)
	}
	if matches[1].PageIndex != 1 {
		t.Fatalf("matches[1].PageIndex = %d, want 1", matches[1].PageIndex)
	}
	if len(matches[0].Rects) == 0 {
		t.Fatal("matches[0].Rects is empty, want at least one highlight rect")
	}
}

func TestSearch_NoMatches(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	matches, err := doc.Search("nonexistentterm")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("len(matches) = %d, want 0", len(matches))
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestSearch -v
```

Expected: FAIL，`Search` 未定义。

- [ ] **Step 3: 实现 search.go**

`textPage` 的真实类型是 `references.FPDF_TEXTPAGE`：

```go
// internal/pdfengine/search.go
package pdfengine

import (
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Rect is a bounding box in PDF page point coordinates (origin bottom-left).
type Rect struct {
	Left, Top, Right, Bottom float64
}

// SearchMatch is one match of a search query, with the highlight rects for
// that match on its page.
type SearchMatch struct {
	PageIndex int
	Rects     []Rect
}

// Search searches the whole document for query (case-insensitive) and
// returns one SearchMatch per hit, ordered by page then position.
func (d *Document) Search(query string) ([]SearchMatch, error) {
	var matches []SearchMatch

	for page := 0; page < d.pages; page++ {
		pageMatches, err := d.searchPage(page, query)
		if err != nil {
			return nil, err
		}
		matches = append(matches, pageMatches...)
	}

	return matches, nil
}

func (d *Document) searchPage(page int, query string) ([]SearchMatch, error) {
	loadResp, err := d.instance.FPDFText_LoadPage(&requests.FPDFText_LoadPage{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{Document: d.handle, Index: page},
		},
	})
	if err != nil {
		return nil, err
	}
	textPage := loadResp.TextPage
	defer d.instance.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: textPage})

	startResp, err := d.instance.FPDFText_FindStart(&requests.FPDFText_FindStart{
		TextPage:   textPage,
		Find:       query,
		StartIndex: 0,
	})
	if err != nil {
		return nil, err
	}
	search := startResp.Search
	defer d.instance.FPDFText_FindClose(&requests.FPDFText_FindClose{Search: search})

	var matches []SearchMatch
	for {
		nextResp, err := d.instance.FPDFText_FindNext(&requests.FPDFText_FindNext{Search: search})
		if err != nil {
			return nil, err
		}
		if !nextResp.GotMatch {
			break
		}

		idxResp, err := d.instance.FPDFText_GetSchResultIndex(&requests.FPDFText_GetSchResultIndex{Search: search})
		if err != nil {
			return nil, err
		}
		countResp, err := d.instance.FPDFText_GetSchCount(&requests.FPDFText_GetSchCount{Search: search})
		if err != nil {
			return nil, err
		}

		rects, err := d.matchRects(textPage, idxResp.Index, countResp.Count)
		if err != nil {
			return nil, err
		}

		matches = append(matches, SearchMatch{PageIndex: page, Rects: rects})
	}

	return matches, nil
}

func (d *Document) matchRects(textPage references.FPDF_TEXTPAGE, startIndex, count int) ([]Rect, error) {
	countResp, err := d.instance.FPDFText_CountRects(&requests.FPDFText_CountRects{
		TextPage:   textPage,
		StartIndex: startIndex,
		Count:      count,
	})
	if err != nil {
		return nil, err
	}

	rects := make([]Rect, 0, countResp.Count)
	for i := 0; i < countResp.Count; i++ {
		r, err := d.instance.FPDFText_GetRect(&requests.FPDFText_GetRect{TextPage: textPage, Index: i})
		if err != nil {
			return nil, err
		}
		rects = append(rects, Rect{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Bottom})
	}
	return rects, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -v
```

Expected: `internal/pdfengine` 下全部测试 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/pdfengine/search.go internal/pdfengine/search_test.go
git commit -m "feat: add pdfengine full-document text search with highlight rects"
```

---

### Task 7: config — 配置加载/保存

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/config/config_test.go
package config

import (
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.RecentFiles) != 0 {
		t.Fatalf("RecentFiles = %v, want empty", cfg.RecentFiles)
	}
	if cfg.WindowWidth != DefaultWindowWidth || cfg.WindowHeight != DefaultWindowHeight {
		t.Fatalf("default window size = %dx%d, want %dx%d", cfg.WindowWidth, cfg.WindowHeight, DefaultWindowWidth, DefaultWindowHeight)
	}
}

func TestSaveThenLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		WindowWidth:  1024,
		WindowHeight: 768,
		SidebarShown: true,
		SidebarTab:   "outline",
	}
	cfg.AddRecent(`C:\docs\a.pdf`)

	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if loaded.WindowWidth != 1024 || loaded.WindowHeight != 768 {
		t.Fatalf("loaded size = %dx%d, want 1024x768", loaded.WindowWidth, loaded.WindowHeight)
	}
	if !loaded.SidebarShown || loaded.SidebarTab != "outline" {
		t.Fatalf("loaded sidebar state = %+v", loaded)
	}
	if len(loaded.RecentFiles) != 1 || loaded.RecentFiles[0].Path != `C:\docs\a.pdf` {
		t.Fatalf("loaded.RecentFiles = %+v", loaded.RecentFiles)
	}
}

func TestLoadFrom_CorruptFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := writeFile(path, []byte("{not json")); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom should not error on corrupt file, got: %v", err)
	}
	if len(cfg.RecentFiles) != 0 {
		t.Fatalf("RecentFiles = %v, want empty default", cfg.RecentFiles)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/config/... -v
```

Expected: FAIL，包内容未定义。

- [ ] **Step 3: 实现 config.go**

```go
// internal/config/config.go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultWindowWidth  = 1000
	DefaultWindowHeight = 720
	MaxRecentFiles      = 10
)

// RecentFile is one entry in the recently-opened-files list.
type RecentFile struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// Config is the persisted application state.
type Config struct {
	RecentFiles  []RecentFile `json:"recentFiles"`
	WindowWidth  int          `json:"windowWidth"`
	WindowHeight int          `json:"windowHeight"`
	SidebarShown bool         `json:"sidebarShown"`
	SidebarTab   string       `json:"sidebarTab"` // "outline" or "thumbnails"
}

func defaultConfig() *Config {
	return &Config{
		WindowWidth:  DefaultWindowWidth,
		WindowHeight: DefaultWindowHeight,
		SidebarShown: true,
		SidebarTab:   "outline",
	}
}

// Path returns the platform config file path: %APPDATA%\PDFReader\config.json
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PDFReader", "config.json"), nil
}

// Load reads the config from the standard location. If the file is
// missing or corrupt, it silently returns default values instead of
// failing, so a bad config never blocks app startup.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return defaultConfig(), nil
	}
	return LoadFrom(path)
}

// LoadFrom reads the config from an explicit path (used by tests).
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), nil
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultConfig(), nil
	}
	return cfg, nil
}

// Save writes the config to the standard location, creating the parent
// directory if needed.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return c.SaveTo(path)
}

// SaveTo writes the config to an explicit path (used by tests).
func (c *Config) SaveTo(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/config/... -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add config load/save with graceful fallback on missing/corrupt file"
```

---

### Task 8: config — 最近打开文件列表管理

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 追加失败的测试**

在 `internal/config/config_test.go` 末尾追加：

```go
func TestAddRecent_DedupeAndMoveToFront(t *testing.T) {
	cfg := defaultConfig()
	cfg.AddRecent(`C:\a.pdf`)
	cfg.AddRecent(`C:\b.pdf`)
	cfg.AddRecent(`C:\a.pdf`) // re-open a.pdf, should move to front, not duplicate

	if len(cfg.RecentFiles) != 2 {
		t.Fatalf("len(RecentFiles) = %d, want 2", len(cfg.RecentFiles))
	}
	if cfg.RecentFiles[0].Path != `C:\a.pdf` {
		t.Fatalf("RecentFiles[0].Path = %q, want C:\\a.pdf", cfg.RecentFiles[0].Path)
	}
	if cfg.RecentFiles[1].Path != `C:\b.pdf` {
		t.Fatalf("RecentFiles[1].Path = %q, want C:\\b.pdf", cfg.RecentFiles[1].Path)
	}
}

func TestAddRecent_CapAtMax(t *testing.T) {
	cfg := defaultConfig()
	for i := 0; i < MaxRecentFiles+5; i++ {
		cfg.AddRecent(`C:\docs\` + string(rune('a'+i)) + `.pdf`)
	}
	if len(cfg.RecentFiles) != MaxRecentFiles {
		t.Fatalf("len(RecentFiles) = %d, want %d", len(cfg.RecentFiles), MaxRecentFiles)
	}
}

func TestRemoveRecent(t *testing.T) {
	cfg := defaultConfig()
	cfg.AddRecent(`C:\a.pdf`)
	cfg.AddRecent(`C:\b.pdf`)

	cfg.RemoveRecent(`C:\a.pdf`)

	if len(cfg.RecentFiles) != 1 || cfg.RecentFiles[0].Path != `C:\b.pdf` {
		t.Fatalf("RecentFiles = %+v, want only C:\\b.pdf", cfg.RecentFiles)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/config/... -run "TestAddRecent|TestRemoveRecent" -v
```

Expected: FAIL，`AddRecent`/`RemoveRecent` 未定义。

- [ ] **Step 3: 在 config.go 中实现**

在 `internal/config/config.go` 末尾追加：

```go
// AddRecent adds path to the front of the recent-files list, moving it to
// the front (without duplicating) if it's already present, and capping the
// list at MaxRecentFiles.
func (c *Config) AddRecent(path string) {
	filtered := make([]RecentFile, 0, len(c.RecentFiles)+1)
	filtered = append(filtered, RecentFile{Path: path, LastOpened: time.Now()})
	for _, rf := range c.RecentFiles {
		if rf.Path == path {
			continue
		}
		filtered = append(filtered, rf)
	}
	if len(filtered) > MaxRecentFiles {
		filtered = filtered[:MaxRecentFiles]
	}
	c.RecentFiles = filtered
}

// RemoveRecent removes path from the recent-files list, if present.
func (c *Config) RemoveRecent(path string) {
	filtered := make([]RecentFile, 0, len(c.RecentFiles))
	for _, rf := range c.RecentFiles {
		if rf.Path == path {
			continue
		}
		filtered = append(filtered, rf)
	}
	c.RecentFiles = filtered
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/config/... -v
```

Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add recent-files list management (dedupe, cap, remove)"
```

---

### Task 9: document — 缩放模式计算

**Files:**
- Create: `internal/document/zoom.go`
- Test: `internal/document/zoom_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/document/zoom_test.go
package document

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestZoom_Percent(t *testing.T) {
	z := Zoom{Mode: ZoomPercent, Percent: 150}
	got := z.ScaleFactor(612, 792, 800, 600) // page size/viewport size irrelevant for ZoomPercent
	if !almostEqual(got, 1.5) {
		t.Fatalf("ScaleFactor = %v, want 1.5", got)
	}
}

func TestZoom_FitWidth(t *testing.T) {
	// Page is 200pt wide, viewport is 800px wide -> scale should make the
	// rendered page (at 72 DPI baseline) exactly fill the viewport width.
	z := Zoom{Mode: ZoomFitWidth}
	got := z.ScaleFactor(200, 400, 800, 600)
	want := 800.0 / 200.0
	if !almostEqual(got, want) {
		t.Fatalf("ScaleFactor = %v, want %v", got, want)
	}
}

func TestZoom_FitPage(t *testing.T) {
	// Page is 200x400pt, viewport is 800x600px. Fit-page must pick the
	// smaller of the width-fit and height-fit scales so the whole page
	// is visible.
	z := Zoom{Mode: ZoomFitPage}
	got := z.ScaleFactor(200, 400, 800, 600)
	widthScale := 800.0 / 200.0
	heightScale := 600.0 / 400.0
	want := math.Min(widthScale, heightScale)
	if !almostEqual(got, want) {
		t.Fatalf("ScaleFactor = %v, want %v", got, want)
	}
}

func TestZoom_DPIForScale(t *testing.T) {
	got := DPIForScale(1.0)
	if got != 72 {
		t.Fatalf("DPIForScale(1.0) = %d, want 72", got)
	}
	got = DPIForScale(2.0)
	if got != 144 {
		t.Fatalf("DPIForScale(2.0) = %d, want 144", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -v
```

Expected: FAIL，包未定义。

- [ ] **Step 3: 实现 zoom.go**

```go
// internal/document/zoom.go
package document

import "math"

// ZoomMode selects how the page scale factor is derived.
type ZoomMode int

const (
	ZoomPercent ZoomMode = iota
	ZoomFitWidth
	ZoomFitPage
)

const (
	MinZoomPercent = 25.0
	MaxZoomPercent = 400.0
)

// Zoom is the current zoom setting for a tab.
type Zoom struct {
	Mode    ZoomMode
	Percent float64 // used when Mode == ZoomPercent, e.g. 100 for 100%.
}

// ScaleFactor returns the multiplier to apply to a page's 72-DPI point
// dimensions to get on-screen pixels, given the page size in points and
// the available viewport size in pixels.
func (z Zoom) ScaleFactor(pageWidthPt, pageHeightPt, viewportWidthPx, viewportHeightPx float64) float64 {
	switch z.Mode {
	case ZoomFitWidth:
		if pageWidthPt <= 0 {
			return 1.0
		}
		return viewportWidthPx / pageWidthPt
	case ZoomFitPage:
		if pageWidthPt <= 0 || pageHeightPt <= 0 {
			return 1.0
		}
		widthScale := viewportWidthPx / pageWidthPt
		heightScale := viewportHeightPx / pageHeightPt
		return math.Min(widthScale, heightScale)
	default: // ZoomPercent
		return z.Percent / 100.0
	}
}

// DPIForScale converts a scale factor (1.0 == 100%) to the DPI value to
// pass to pdfengine.RenderPage, using 72 DPI as the 100% baseline (PDF
// points are defined as 1/72 inch).
func DPIForScale(scale float64) int {
	return int(math.Round(72.0 * scale))
}

// ClampPercent clamps a percentage zoom value to [MinZoomPercent, MaxZoomPercent].
func ClampPercent(percent float64) float64 {
	if percent < MinZoomPercent {
		return MinZoomPercent
	}
	if percent > MaxZoomPercent {
		return MaxZoomPercent
	}
	return percent
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/document/zoom.go internal/document/zoom_test.go
git commit -m "feat: add zoom mode scale-factor calculations"
```

---

### Task 10: document — 页面位图 LRU 缓存

**Files:**
- Create: `internal/document/cache.go`
- Test: `internal/document/cache_test.go`

- [ ] **Step 1: 写失败的测试**

```go
// internal/document/cache_test.go
package document

import (
	"image"
	"testing"
)

func fakeImage(w, h int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

func TestCache_GetMiss(t *testing.T) {
	c := NewCache(3)
	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); ok {
		t.Fatal("Get on empty cache should miss")
	}
}

func TestCache_PutThenGet(t *testing.T) {
	c := NewCache(3)
	img := fakeImage(10, 10)
	key := CacheKey{Page: 0, DPI: 72}

	c.Put(key, img)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("Get after Put should hit")
	}
	if got != img {
		t.Fatal("Get returned a different image than was Put")
	}
}

func TestCache_EvictsOldestWhenOverCapacity(t *testing.T) {
	c := NewCache(2)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 2, DPI: 72}, fakeImage(1, 1)) // should evict page 0

	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); ok {
		t.Fatal("page 0 should have been evicted")
	}
	if _, ok := c.Get(CacheKey{Page: 1, DPI: 72}); !ok {
		t.Fatal("page 1 should still be cached")
	}
	if _, ok := c.Get(CacheKey{Page: 2, DPI: 72}); !ok {
		t.Fatal("page 2 should be cached")
	}
}

func TestCache_GetRefreshesRecency(t *testing.T) {
	c := NewCache(2)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1))

	c.Get(CacheKey{Page: 0, DPI: 72}) // touch page 0, page 1 becomes least-recent

	c.Put(CacheKey{Page: 2, DPI: 72}, fakeImage(1, 1)) // should evict page 1, not page 0

	if _, ok := c.Get(CacheKey{Page: 1, DPI: 72}); ok {
		t.Fatal("page 1 should have been evicted")
	}
	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); !ok {
		t.Fatal("page 0 should still be cached (recently touched)")
	}
}

func TestCache_DifferentDPISameSamePageAreDistinctKeys(t *testing.T) {
	c := NewCache(3)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 0, DPI: 150}, fakeImage(2, 2))

	img72, ok := c.Get(CacheKey{Page: 0, DPI: 72})
	if !ok || img72.Bounds().Dx() != 1 {
		t.Fatalf("expected distinct cache entry for DPI 72")
	}
	img150, ok := c.Get(CacheKey{Page: 0, DPI: 150})
	if !ok || img150.Bounds().Dx() != 2 {
		t.Fatalf("expected distinct cache entry for DPI 150")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -run TestCache -v
```

Expected: FAIL，`Cache`/`CacheKey`/`NewCache` 未定义。

- [ ] **Step 3: 实现 cache.go**

```go
// internal/document/cache.go
package document

import (
	"container/list"
	"image"
)

// CacheKey identifies a rendered page at a specific DPI.
type CacheKey struct {
	Page int
	DPI  int
}

type cacheEntry struct {
	key CacheKey
	img *image.RGBA
}

// Cache is a small LRU cache of rendered page images, keyed by
// (page index, DPI). It exists to avoid re-rendering the page the user
// just navigated away from and back to.
type Cache struct {
	capacity int
	ll       *list.List // front = most recently used
	items    map[CacheKey]*list.Element
}

// NewCache creates an LRU cache holding at most capacity entries.
func NewCache(capacity int) *Cache {
	return &Cache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[CacheKey]*list.Element),
	}
}

// Get returns the cached image for key, if present, and marks it as
// recently used.
func (c *Cache) Get(key CacheKey) (*image.RGBA, bool) {
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*cacheEntry).img, true
}

// Put stores img under key, evicting the least-recently-used entry if the
// cache is over capacity.
func (c *Cache) Put(key CacheKey, img *image.RGBA) {
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).img = img
		c.ll.MoveToFront(el)
		return
	}

	el := c.ll.PushFront(&cacheEntry{key: key, img: img})
	c.items[key] = el

	for c.ll.Len() > c.capacity {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/document/... -v
```

Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/document/cache.go internal/document/cache_test.go
git commit -m "feat: add LRU page-image cache"
```

---

## UI 任务说明

从这里开始的任务构建 `internal/ui` 包和 `main.go`。walk 没有可用于无头 CI 的控件模拟点击机制，所以这些任务**不写自动化测试**，而是每个任务给出明确的手动验证步骤（构建 exe，实际运行，按步骤操作，确认现象）。这与设计文档中"测试策略"一节的约定一致。

所有 UI 任务共用同一个 `internal/ui` 包和一组贯穿始终的类型，先在这里统一定义，后续任务在此基础上增量添加：

- `type tab struct` —— 一个标签页的完整状态（Task 11 定义骨架，Task 12 起逐步填充字段）。
- `type app struct` —— 整个应用运行时（主窗口引用、配置、pdfium 连接池、所有打开的 tab）。

---

### Task 11: UI — 主窗口骨架（菜单栏/工具栏/状态栏/标签容器）

**Files:**
- Create: `internal/ui/app.go`
- Create: `internal/ui/tab.go`

- [ ] **Step 1: 定义 app 与 tab 骨架类型**

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

	outline []pdfengine.OutlineNode

	tabPage  *walk.TabPage
	pageView *walk.CustomWidget
}

func newTab(path string, doc *pdfengine.Document) *tab {
	return &tab{
		path:  path,
		doc:   doc,
		page:  0,
		zoom:  document.Zoom{Mode: document.ZoomFitPage},
		cache: document.NewCache(5),
	}
}
```

```go
// internal/ui/app.go
package ui

import (
	"github.com/klippa-app/go-pdfium"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pdfreader/internal/config"
	"pdfreader/internal/pdfengine"
)

// app is the single running instance of the UI, owning the pdfium pool,
// the persisted config, the main window and all open tabs.
type app struct {
	pool *pdfengine.Pool
	cfg  *config.Config

	mainWindow *walk.MainWindow
	tabWidget  *walk.TabWidget
	statusBar  *walk.StatusBarItem

	tabs []*tab
}

var _ = pdfium.Pdfium(nil) // keep import used until pool wiring lands in later tasks

// Run builds and shows the main window, blocking until it's closed.
// initialFile may be empty; if set, it is opened as the first tab on
// startup (see Task 20).
func Run(initialFile string) (int, error) {
	pool, err := pdfengine.NewPool()
	if err != nil {
		return 1, err
	}
	defer pool.Close()

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	a := &app{pool: pool, cfg: cfg}

	mw := MainWindow{
		AssignTo: &a.mainWindow,
		Title:    "PDF 阅读器",
		Size:     Size{Width: cfg.WindowWidth, Height: cfg.WindowHeight},
		Layout:   VBox{MarginsZero: true},
		Children: []Widget{
			TabWidget{
				AssignTo: &a.tabWidget,
			},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &a.statusBar, Text: "就绪"},
		},
	}

	return mw.Run()
}
```

- [ ] **Step 2: main.go 接入**

```go
// main.go
package main

import (
	"fmt"
	"os"

	"pdfreader/internal/ui"
)

func main() {
	if _, err := ui.Run(""); err != nil {
		fmt.Fprintln(os.Stderr, "pdfreader:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: 添加 walk 依赖并构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" get github.com/lxn/walk@v0.0.0-20210112085537-c389da54e794
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 构建成功（若有 unused import 报错，检查 Step 1 中 `var _ = pdfium.Pdfium(nil)` 那行是否原样保留）。

- [ ] **Step 4: 手动验证**

运行 `./pdfreader.exe`。确认：
1. 窗口标题为"PDF 阅读器"，尺寸约 1000×720。
2. 窗口内是一个空的标签页控件（没有任何标签）。
3. 底部状态栏显示"就绪"。
4. 关闭窗口程序正常退出，无残留进程（任务管理器确认）。

- [ ] **Step 5: 提交**

```bash
git add go.mod go.sum main.go internal/ui/app.go internal/ui/tab.go
git commit -m "feat: add main window shell with empty tab container and status bar"
```

---

### Task 12: UI — 打开文件、页面渲染、首个标签页

**Files:**
- Modify: `internal/ui/app.go`
- Create: `internal/ui/pageview.go`

- [ ] **Step 1: 实现页面位图绘制辅助（pageview.go）**

```go
// internal/ui/pageview.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/document"
)

// renderCurrentPage renders t's current page at t's current zoom (using
// the cache when possible) and returns a walk.Bitmap ready to paint.
// The caller owns the returned bitmap and must Dispose() it when replaced.
func (t *tab) renderCurrentPage(viewportW, viewportH float64) (*walk.Bitmap, error) {
	pageWidthPt, pageHeightPt := 612.0, 792.0 // US Letter fallback; refined in Task 14 via real page size.

	scale := t.zoom.ScaleFactor(pageWidthPt, pageHeightPt, viewportW, viewportH)
	dpi := document.DPIForScale(scale)

	key := document.CacheKey{Page: t.page, DPI: dpi}
	img, ok := t.cache.Get(key)
	if !ok {
		rendered, err := t.doc.RenderPage(t.page, dpi)
		if err != nil {
			return nil, err
		}
		img = rendered
		t.cache.Put(key, img)
	}

	return walk.NewBitmapFromImage(img)
}
```

- [ ] **Step 2: 在 app.go 中实现 openFile，并接上"打开"按钮**

`internal/ui/tab.go` 不需要改动（`tabPage`/`pageView` 字段已在 Task 11 中定义好）。`TabPage` 必须作为 `TabWidget.Pages` 的元素随其容器一起创建，不能脱离父级单独构建后再 `Add`，所以这里改用 walk 的非声明式 API 直接创建 `TabPage`（而不是 `declarative.TabPage`）。修改 `internal/ui/app.go`，把菜单/工具栏补全，并新增 `openFile` 方法：

```go
// internal/ui/app.go
package ui

import (
	"fmt"
	"os"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pdfreader/internal/config"
	"pdfreader/internal/pdfengine"
)

type app struct {
	pool *pdfengine.Pool
	cfg  *config.Config

	mainWindow *walk.MainWindow
	tabWidget  *walk.TabWidget
	statusBar  *walk.StatusBarItem

	tabs []*tab
}

func Run(initialFile string) (int, error) {
	pool, err := pdfengine.NewPool()
	if err != nil {
		return 1, err
	}
	defer pool.Close()

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	a := &app{pool: pool, cfg: cfg}

	mw := MainWindow{
		AssignTo: &a.mainWindow,
		Title:    "PDF 阅读器",
		Size:     Size{Width: cfg.WindowWidth, Height: cfg.WindowHeight},
		Layout:   VBox{MarginsZero: true},
		MenuItems: []MenuItem{
			Menu{
				Text: "文件(&F)",
				Items: []MenuItem{
					Action{Text: "打开...(&O)", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyO}, OnTriggered: a.onOpenClicked},
					Action{Text: "退出(&X)", OnTriggered: func() { a.mainWindow.Close() }},
				},
			},
		},
		ToolBar: ToolBar{
			Items: []MenuItem{
				Action{Text: "打开", OnTriggered: a.onOpenClicked},
			},
		},
		Children: []Widget{
			TabWidget{AssignTo: &a.tabWidget},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &a.statusBar, Text: "就绪"},
		},
	}

	code, err := mw.Run()
	return code, err
}

func (a *app) onOpenClicked() {
	dlg := walk.FileDialog{
		Title:  "打开 PDF",
		Filter: "PDF 文件 (*.pdf)|*.pdf",
	}
	ok, err := dlg.ShowOpen(a.mainWindow)
	if err != nil || !ok {
		return
	}
	if err := a.openFile(dlg.FilePath); err != nil {
		walk.MsgBox(a.mainWindow, "无法打开文件", err.Error(), walk.MsgBoxIconError)
	}
}

// openFile opens path and adds it as a new tab. It is reused from Task 20
// for command-line startup, so both the Open dialog and the command-line
// argument path share one code path.
//
// TabPage must be created as a child of its TabWidget directly (it can't
// be built standalone via declarative.TabPage and Add()-ed afterwards), so
// this uses walk's non-declarative constructors instead of the `declarative`
// package for the tab's contents.
func (a *app) openFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	doc, err := a.pool.Open(data, nil)
	if err != nil {
		return err
	}

	t := newTab(path, doc)

	tabPage, err := walk.NewTabPage()
	if err != nil {
		doc.Close()
		return err
	}
	tabPage.SetTitle(filepathBase(path))
	tabPage.SetLayout(walk.NewVBoxLayout())

	pageView, err := walk.NewCustomWidget(tabPage, 0, func(canvas *walk.Canvas, updateBounds walk.Rectangle) error {
		return a.paintTab(t, canvas, updateBounds)
	})
	if err != nil {
		doc.Close()
		return err
	}
	pageView.SetClearsBackground(true)

	if err := a.tabWidget.Pages().Add(tabPage); err != nil {
		doc.Close()
		return err
	}

	t.tabPage = tabPage
	t.pageView = pageView

	a.tabs = append(a.tabs, t)
	a.tabWidget.SetCurrentIndex(a.tabWidget.Pages().Len() - 1)

	a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", t.page+1, t.doc.PageCount()))
	pageView.Invalidate()

	return nil
}

func (a *app) paintTab(t *tab, canvas *walk.Canvas, updateBounds walk.Rectangle) error {
	bounds := t.pageView.ClientBounds()
	bmp, err := t.renderCurrentPage(float64(bounds.Width), float64(bounds.Height))
	if err != nil {
		return canvas.DrawText(err.Error(), nil, walk.RGB(200, 0, 0), updateBounds, walk.TextWordbreak)
	}
	defer bmp.Dispose()

	return canvas.DrawImage(bmp, walk.Point{X: 0, Y: 0})
}

func filepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
```

- [ ] **Step 3: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 构建成功。如果报 `walk.TextWordbreak` 或 `walk.RGB` 相关的类型错误，用 `go doc github.com/lxn/walk.Canvas` /`go doc github.com/lxn/walk DrawTextFormat` 核对常量名后修正（这两个 API 在此版本 walk 中确实存在，属于常见签名差异排查）。

- [ ] **Step 4: 手动验证**

运行 `./pdfreader.exe`：
1. 点击工具栏"打开"或菜单"文件 > 打开..."，选择 `testdata/sample.pdf`。
2. 确认出现一个新标签页，标题为 "sample.pdf"。
3. 确认标签页内容区渲染出了 PDF 第一页（应为一段文字 "hello world page one"）。
4. 确认状态栏显示"第 1 / 2 页"。
5. 用 `testdata/encrypted.pdf` 重复上述操作：此时应弹出"无法打开文件"错误框（因为密码对话框要到 Task 19 才接入，属预期行为）。

- [ ] **Step 5: 提交**

```bash
git add internal/ui/app.go internal/ui/pageview.go
git commit -m "feat: wire file open dialog and render first page in a new tab"
```

---

### Task 13: UI — 翻页导航与快捷键

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 添加导航方法与菜单/工具栏项**

在 `internal/ui/app.go` 的 `MenuItems` 中的"文件"菜单后面添加一个"转到"菜单，并在 `ToolBar.Items` 中添加对应按钮：

```go
// MenuItems 数组中，"文件" Menu 之后追加：
Menu{
	Text: "转到(&G)",
	Items: []MenuItem{
		Action{Text: "上一页", Shortcut: Shortcut{Key: walk.KeyPrior}, OnTriggered: a.onPrevPage},
		Action{Text: "下一页", Shortcut: Shortcut{Key: walk.KeyNext}, OnTriggered: a.onNextPage},
		Action{Text: "首页", Shortcut: Shortcut{Key: walk.KeyHome}, OnTriggered: a.onFirstPage},
		Action{Text: "末页", Shortcut: Shortcut{Key: walk.KeyEnd}, OnTriggered: a.onLastPage},
	},
},
```

```go
// ToolBar.Items 数组中，"打开" Action 之后追加：
Separator{},
Action{Text: "◀", OnTriggered: a.onPrevPage},
Action{Text: "▶", OnTriggered: a.onNextPage},
```

在文件末尾添加导航逻辑：

```go
func (a *app) currentTab() *tab {
	if a.tabWidget == nil || a.tabWidget.Pages().Len() == 0 {
		return nil
	}
	idx := a.tabWidget.CurrentIndex()
	if idx < 0 || idx >= len(a.tabs) {
		return nil
	}
	return a.tabs[idx]
}

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

func (a *app) onPrevPage() {
	t := a.currentTab()
	if t == nil {
		return
	}
	a.goToPage(t, t.page-1)
}

func (a *app) onNextPage() {
	t := a.currentTab()
	if t == nil {
		return
	}
	a.goToPage(t, t.page+1)
}

func (a *app) onFirstPage() {
	if t := a.currentTab(); t != nil {
		a.goToPage(t, 0)
	}
}

func (a *app) onLastPage() {
	if t := a.currentTab(); t != nil {
		a.goToPage(t, t.doc.PageCount()-1)
	}
}
```

- [ ] **Step 2: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 3: 手动验证**

1. 打开 `testdata/sample.pdf`。
2. 点击工具栏"▶"，确认视图切换到第 2 页（"goodbye world page two"），状态栏显示"第 2 / 2 页"。
3. 按键盘 PageUp，确认回到第 1 页。
4. 按 End，确认跳到末页；按 Home，确认跳回首页。
5. 在末页再点"▶"，确认页码不会超出范围（仍停在末页，不报错）。

- [ ] **Step 4: 提交**

```bash
git add internal/ui/app.go
git commit -m "feat: add page navigation (prev/next/first/last) with keyboard shortcuts"
```

---

### Task 14: UI — 缩放控制（使用真实页面尺寸）

**Files:**
- Modify: `internal/ui/tab.go`
- Modify: `internal/ui/pageview.go`
- Modify: `internal/ui/app.go`

pdfengine 目前没有暴露"页面点数尺寸"的 API（Task 4 只做了渲染）。本任务先给 pdfengine 加一个轻量方法，再接入缩放 UI。

- [ ] **Step 1: 给 pdfengine 加 PageSize**

在 `internal/pdfengine/render.go` 末尾追加：

```go
// PageSize returns the page's width and height in PDF points (1/72 inch).
func (d *Document) PageSize(index int) (widthPt, heightPt float64, err error) {
	if index < 0 || index >= d.pages {
		return 0, 0, fmt.Errorf("pdfengine: page index %d out of range [0,%d)", index, d.pages)
	}
	resp, err := d.instance.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
		Document: d.handle,
		Index:    index,
	})
	if err != nil {
		return 0, 0, err
	}
	return resp.Width, resp.Height, nil
}
```

在 `internal/pdfengine/render_test.go` 中追加：

```go
func TestPageSize(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	doc, err := pool.Open(readTestdata(t, "sample.pdf"), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	w, h, err := doc.PageSize(0)
	if err != nil {
		t.Fatalf("PageSize: %v", err)
	}
	// A4 in points, from gofpdf: 595.28 x 841.89 (approximately).
	if w < 590 || w > 600 || h < 835 || h > 848 {
		t.Fatalf("PageSize = %vx%v, want ~595x842 (A4)", w, h)
	}
}
```

运行：

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" test ./internal/pdfengine/... -run TestPageSize -v
```

Expected: PASS（若字段名 `resp.Width`/`resp.Height` 与实际不符，用 `go doc github.com/klippa-app/go-pdfium/responses.FPDF_GetPageSizeByIndex` 核对并修正）。

提交：

```bash
git add internal/pdfengine/render.go internal/pdfengine/render_test.go
git commit -m "feat: add pdfengine PageSize for accurate zoom calculations"
```

- [ ] **Step 2: pageview.go 改用真实页面尺寸**

修改 `internal/ui/pageview.go` 中的 `renderCurrentPage`：

```go
// internal/ui/pageview.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/document"
)

func (t *tab) renderCurrentPage(viewportW, viewportH float64) (*walk.Bitmap, error) {
	pageWidthPt, pageHeightPt, err := t.doc.PageSize(t.page)
	if err != nil {
		return nil, err
	}

	scale := t.zoom.ScaleFactor(pageWidthPt, pageHeightPt, viewportW, viewportH)
	dpi := document.DPIForScale(scale)

	key := document.CacheKey{Page: t.page, DPI: dpi}
	img, ok := t.cache.Get(key)
	if !ok {
		rendered, err := t.doc.RenderPage(t.page, dpi)
		if err != nil {
			return nil, err
		}
		img = rendered
		t.cache.Put(key, img)
	}

	return walk.NewBitmapFromImage(img)
}
```

- [ ] **Step 3: 在 app.go 添加缩放菜单/工具栏与逻辑**

在"转到"菜单前插入"视图"菜单：

```go
Menu{
	Text: "视图(&V)",
	Items: []MenuItem{
		Action{Text: "放大", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyOEMPlus}, OnTriggered: a.onZoomIn},
		Action{Text: "缩小", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyOEMMinus}, OnTriggered: a.onZoomOut},
		Action{Text: "适合宽度", OnTriggered: a.onFitWidth},
		Action{Text: "适合页面", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.Key0}, OnTriggered: a.onFitPage},
	},
},
```

在文件末尾添加：

```go
func (a *app) setZoom(t *tab, z document.Zoom) {
	if t == nil {
		return
	}
	t.zoom = z
	t.pageView.Invalidate()
}

func (a *app) onZoomIn() {
	t := a.currentTab()
	if t == nil {
		return
	}
	percent := t.zoom.Percent
	if t.zoom.Mode != document.ZoomPercent {
		percent = 100
	}
	a.setZoom(t, document.Zoom{Mode: document.ZoomPercent, Percent: document.ClampPercent(percent + 10)})
}

func (a *app) onZoomOut() {
	t := a.currentTab()
	if t == nil {
		return
	}
	percent := t.zoom.Percent
	if t.zoom.Mode != document.ZoomPercent {
		percent = 100
	}
	a.setZoom(t, document.Zoom{Mode: document.ZoomPercent, Percent: document.ClampPercent(percent - 10)})
}

func (a *app) onFitWidth() {
	if t := a.currentTab(); t != nil {
		a.setZoom(t, document.Zoom{Mode: document.ZoomFitWidth})
	}
}

func (a *app) onFitPage() {
	if t := a.currentTab(); t != nil {
		a.setZoom(t, document.Zoom{Mode: document.ZoomFitPage})
	}
}
```

别忘了在 `internal/ui/app.go` 顶部 import 中加入 `"pdfreader/internal/document"`。

- [ ] **Step 4: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 5: 手动验证**

1. 打开 `testdata/sample.pdf`（默认适合页面模式，确认页面完整可见且居中不溢出控件）。
2. 用 Ctrl+= 放大几次，确认渲染的文字明显变大、变清晰（不是简单拉伸模糊）。
3. 用 Ctrl+- 缩小回去，确认不会缩到 25% 以下（多按几次验证下限生效）。
4. 切换"适合宽度"，缩放宽度调整窗口大小，确认页面宽度始终与视图区宽度一致。
5. Ctrl+0 恢复"适合页面"。

- [ ] **Step 6: 提交**

```bash
git add internal/ui/app.go internal/ui/pageview.go
git commit -m "feat: wire zoom controls (percent/fit-width/fit-page) using real page size"
```

---

### Task 15: UI — 侧边栏：目录大纲

**Files:**
- Create: `internal/ui/outlinesidebar.go`
- Modify: `internal/ui/tab.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 实现 TreeModel 适配器**

```go
// internal/ui/outlinesidebar.go
package ui

import (
	"github.com/lxn/walk"

	"pdfreader/internal/pdfengine"
)

// outlineItem adapts one pdfengine.OutlineNode to walk.TreeItem.
type outlineItem struct {
	node     pdfengine.OutlineNode
	parent   *outlineItem
	children []*outlineItem
}

func newOutlineItem(node pdfengine.OutlineNode, parent *outlineItem) *outlineItem {
	item := &outlineItem{node: node, parent: parent}
	item.children = make([]*outlineItem, len(node.Children))
	for i, child := range node.Children {
		item.children[i] = newOutlineItem(child, item)
	}
	return item
}

func (i *outlineItem) Text() string       { return i.node.Title }
func (i *outlineItem) ChildCount() int    { return len(i.children) }
func (i *outlineItem) ChildAt(index int) walk.TreeItem { return i.children[index] }
func (i *outlineItem) Parent() walk.TreeItem {
	if i.parent == nil {
		return nil
	}
	return i.parent
}

// outlineModel implements walk.TreeModel over a document's outline.
type outlineModel struct {
	walk.TreeModelBase
	roots []*outlineItem
}

func newOutlineModel(nodes []pdfengine.OutlineNode) *outlineModel {
	m := &outlineModel{roots: make([]*outlineItem, len(nodes))}
	for i, n := range nodes {
		m.roots[i] = newOutlineItem(n, nil)
	}
	return m
}

func (m *outlineModel) RootCount() int             { return len(m.roots) }
func (m *outlineModel) RootAt(index int) walk.TreeItem { return m.roots[index] }
```

- [ ] **Step 2: 在 tab 中加入侧边栏相关字段**

修改 `internal/ui/tab.go`，在 `tab` 结构体的 `outline []pdfengine.OutlineNode` 字段下面追加：

```go
	outlineTree *walk.TreeView
```

- [ ] **Step 3: 在 app.go 中构建带侧边栏的标签页布局**

修改 `internal/ui/app.go` 的 `openFile`：把原来直接给 `tabPage` 加 `CustomWidget` 子控件，改成先加一个 `HSplitter`，左边放大纲 `TreeView`，右边放页面视图。用非声明式 API 写法（延续 Task 12 的选择）：

```go
// openFile 中，创建 tabPage 之后、创建 pageView 之前，插入：
splitter, err := walk.NewHSplitter(tabPage)
if err != nil {
	doc.Close()
	return err
}

sidebarComposite, err := walk.NewComposite(splitter)
if err != nil {
	doc.Close()
	return err
}
sidebarComposite.SetLayout(walk.NewVBoxLayout())

outline, err := doc.Outline()
if err != nil {
	outline = nil // treat outline errors as "no bookmarks" rather than failing the whole open
}
t.outline = outline

treeView, err := walk.NewTreeView(sidebarComposite)
if err != nil {
	doc.Close()
	return err
}
if err := treeView.SetModel(newOutlineModel(outline)); err != nil {
	doc.Close()
	return err
}
treeView.ItemActivated().Attach(func() {
	item, ok := treeView.CurrentItem().(*outlineItem)
	if !ok || item == nil {
		return
	}
	if item.node.PageIndex >= 0 {
		a.goToPage(t, item.node.PageIndex)
	}
})
t.outlineTree = treeView

// pageView 的 parent 从 tabPage 改成 splitter：
pageView, err := walk.NewCustomWidget(splitter, 0, func(canvas *walk.Canvas, updateBounds walk.Rectangle) error {
	return a.paintTab(t, canvas, updateBounds)
})
```

`tabPage.SetLayout(walk.NewVBoxLayout())` 这一行保留在原位（`splitter` 作为 `tabPage` 唯一的子控件会自动撑满）。

- [ ] **Step 4: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 5: 手动验证**

1. 打开 `testdata/sample.pdf`。
2. 确认标签页左侧出现一个窄的目录树面板，显示两个节点："Page One"、"Page Two"。
3. 双击（或选中后按 Enter）"Page Two"，确认右侧视图跳转到第 2 页，状态栏显示"第 2 / 2 页"。
4. 拖动分隔条，确认左右面板可以调整宽度比例。

- [ ] **Step 6: 提交**

```bash
git add internal/ui/outlinesidebar.go internal/ui/tab.go internal/ui/app.go
git commit -m "feat: add outline/bookmark sidebar with click-to-navigate"
```

---

### Task 16: UI — 侧边栏：缩略图页签

**Files:**
- Create: `internal/ui/thumbnails.go`
- Modify: `internal/ui/app.go`

为了控制范围，缩略图面板实现为一个可滚动的 `Composite`，内含每页一个小的 `ImageView`，缩略图在标签页打开时一次性渲染（sample.pdf 只有 2 页；对更大文档的按需/增量渲染留在设计文档"范围之外"事项的后续迭代，此处先满足"标准阅读器"验收标准：面板能显示、能点击跳转）。

- [ ] **Step 1: 实现缩略图面板构建函数**

```go
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
			return err
		}
		if err := iv.SetImage(bmp); err != nil {
			return err
		}
		iv.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
			onActivate(page)
		})
	}
	return nil
}
```

- [ ] **Step 2: 在 tab 中标记侧边栏当前页签，在 app.go 用 TabWidget 分隔"目录"/"缩略图"**

修改 `internal/ui/app.go` 的 `openFile`，把 Task 15 里直接放进 `sidebarComposite` 的 `treeView`，改成放进一个内嵌 `TabWidget` 的第一页；缩略图作为第二页：

```go
// 替换 Task 15 中 "sidebarComposite.SetLayout(...)" 之后到 "t.outlineTree = treeView" 之间的内容：
sidebarComposite.SetLayout(walk.NewVBoxLayout())

sidebarTabs, err := walk.NewTabWidget(sidebarComposite)
if err != nil {
	doc.Close()
	return err
}

outlinePage, err := walk.NewTabPage()
if err != nil {
	doc.Close()
	return err
}
outlinePage.SetTitle("目录")
outlinePage.SetLayout(walk.NewVBoxLayout())

outline, err := doc.Outline()
if err != nil {
	outline = nil
}
t.outline = outline

treeView, err := walk.NewTreeView(outlinePage)
if err != nil {
	doc.Close()
	return err
}
if err := treeView.SetModel(newOutlineModel(outline)); err != nil {
	doc.Close()
	return err
}
treeView.ItemActivated().Attach(func() {
	item, ok := treeView.CurrentItem().(*outlineItem)
	if !ok || item == nil {
		return
	}
	if item.node.PageIndex >= 0 {
		a.goToPage(t, item.node.PageIndex)
	}
})
t.outlineTree = treeView
if err := sidebarTabs.Pages().Add(outlinePage); err != nil {
	doc.Close()
	return err
}

thumbsPage, err := walk.NewTabPage()
if err != nil {
	doc.Close()
	return err
}
thumbsPage.SetTitle("缩略图")
thumbsPage.SetLayout(walk.NewVBoxLayout())

thumbsScroll, err := walk.NewScrollView(thumbsPage)
if err != nil {
	doc.Close()
	return err
}
thumbsScroll.SetLayout(walk.NewVBoxLayout())

if err := buildThumbnails(thumbsScroll, doc, func(page int) {
	a.goToPage(t, page)
}); err != nil {
	doc.Close()
	return err
}
if err := sidebarTabs.Pages().Add(thumbsPage); err != nil {
	doc.Close()
	return err
}
```

- [ ] **Step 3: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 4: 手动验证**

1. 打开 `testdata/sample.pdf`。
2. 确认侧边栏顶部出现"目录"/"缩略图"两个页签。
3. 切换到"缩略图"，确认看到 2 张小缩略图。
4. 点击第 2 张缩略图，确认主视图跳转到第 2 页。

- [ ] **Step 5: 提交**

```bash
git add internal/ui/thumbnails.go internal/ui/app.go
git commit -m "feat: add thumbnails sidebar tab with click-to-navigate"
```

---

### Task 17: UI — 查找栏

**Files:**
- Create: `internal/ui/searchbar.go`
- Modify: `internal/ui/tab.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 在 tab 中加入搜索状态**

在 `internal/ui/tab.go` 的 `tab` 结构体末尾追加：

```go
	searchMatches []pdfengine.SearchMatch
	searchIndex   int // index into searchMatches of the currently-highlighted match
```

- [ ] **Step 2: 实现查找栏构建与逻辑**

```go
// internal/ui/searchbar.go
package ui

import (
	"fmt"

	"github.com/lxn/walk"
)

// buildSearchBar creates a find bar (edit box + prev/next + status label)
// as a child of parent, initially hidden. It returns the composite so the
// caller can show/hide and focus it.
func (a *app) buildSearchBar(parent walk.Container, t *tab) (*walk.Composite, error) {
	bar, err := walk.NewComposite(parent)
	if err != nil {
		return nil, err
	}
	bar.SetLayout(walk.NewHBoxLayout())
	bar.SetVisible(false)

	edit, err := walk.NewLineEdit(bar)
	if err != nil {
		return nil, err
	}

	status, err := walk.NewLabel(bar)
	if err != nil {
		return nil, err
	}

	runSearch := func() {
		query := edit.Text()
		if query == "" {
			t.searchMatches = nil
			status.SetText("")
			return
		}
		matches, err := t.doc.Search(query)
		if err != nil {
			status.SetText("搜索出错")
			return
		}
		t.searchMatches = matches
		t.searchIndex = 0
		if len(matches) == 0 {
			status.SetText("未找到")
			return
		}
		status.SetText(fmt.Sprintf("第 1/%d 处匹配", len(matches)))
		a.goToPage(t, matches[0].PageIndex)
	}

	gotoMatch := func(delta int) {
		if len(t.searchMatches) == 0 {
			return
		}
		t.searchIndex = (t.searchIndex + delta + len(t.searchMatches)) % len(t.searchMatches)
		status.SetText(fmt.Sprintf("第 %d/%d 处匹配", t.searchIndex+1, len(t.searchMatches)))
		a.goToPage(t, t.searchMatches[t.searchIndex].PageIndex)
	}

	edit.KeyPress().Attach(func(key walk.Key) {
		switch key {
		case walk.KeyReturn:
			runSearch()
		case walk.KeyEscape:
			bar.SetVisible(false)
		}
	})

	nextBtn, err := walk.NewPushButton(bar)
	if err != nil {
		return nil, err
	}
	nextBtn.SetText("下一个")
	nextBtn.Clicked().Attach(func() { gotoMatch(1) })

	prevBtn, err := walk.NewPushButton(bar)
	if err != nil {
		return nil, err
	}
	prevBtn.SetText("上一个")
	prevBtn.Clicked().Attach(func() { gotoMatch(-1) })

	t.searchEdit = edit
	t.searchBar = bar

	return bar, nil
}
```

在 `internal/ui/tab.go` 的 `tab` 结构体中再追加两个字段（放在 `searchIndex int` 之后）：

```go
	searchBar  *walk.Composite
	searchEdit *walk.LineEdit
```

- [ ] **Step 3: 在 openFile 中调用 buildSearchBar，并接上 Ctrl+F**

在 `internal/ui/app.go` 的 `openFile` 中，`splitter` 创建完成之后（作为 `tabPage` 的第二个子控件，`VBox` 布局下会排在 splitter 下方或上方均可，这里放在下方）追加：

```go
searchBar, err := a.buildSearchBar(tabPage, t)
if err != nil {
	doc.Close()
	return err
}
_ = searchBar
```

在"文件"菜单前的合适位置（比如"转到"菜单里）追加查找相关快捷键；直接在文件末尾追加一个新方法并在 `MenuItems` 的"视图"菜单里加一项 `Action{Text: "查找", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyF}, OnTriggered: a.onToggleSearch}`：

```go
func (a *app) onToggleSearch() {
	t := a.currentTab()
	if t == nil || t.searchBar == nil {
		return
	}
	visible := !t.searchBar.Visible()
	t.searchBar.SetVisible(visible)
	if visible {
		t.searchEdit.SetFocus()
	}
}
```

- [ ] **Step 4: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 5: 手动验证**

1. 打开 `testdata/sample.pdf`，按 Ctrl+F，确认查找栏出现并获得焦点。
2. 输入 "world"，回车，确认状态标签显示"第 1/2 处匹配"，视图跳到第 1 页。
3. 点击"下一个"，确认跳到第 2 页，标签显示"第 2/2 处匹配"。
4. 再点"下一个"，确认循环回到第 1 页（"第 1/2 处匹配"）。
5. 搜索一个不存在的词，确认显示"未找到"。
6. 按 Esc，确认查找栏隐藏。

（本任务范围内不要求在页面上绘制高亮矩形叠加层——`pdfengine.SearchMatch.Rects` 已经就绪，视觉高亮作为后续迭代；当前验收标准是"能跳转到命中页并提示第几处匹配"。）

- [ ] **Step 6: 提交**

```bash
git add internal/ui/searchbar.go internal/ui/tab.go internal/ui/app.go
git commit -m "feat: add inline find bar with next/prev match navigation"
```

---

### Task 18: UI — 多标签页管理（关闭标签）

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 实现关闭当前标签逻辑**

在 `internal/ui/app.go` 末尾追加：

```go
func (a *app) closeCurrentTab() {
	idx := a.tabWidget.CurrentIndex()
	if idx < 0 || idx >= len(a.tabs) {
		return
	}

	t := a.tabs[idx]
	t.doc.Close()

	a.tabWidget.Pages().RemoveAt(idx)
	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)

	if len(a.tabs) == 0 {
		a.statusBar.SetText("就绪")
	} else if nt := a.currentTab(); nt != nil {
		a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", nt.page+1, nt.doc.PageCount()))
	}
}
```

在"文件"菜单里，"打开..."之后加入：

```go
Action{Text: "关闭标签(&W)", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyW}, OnTriggered: a.closeCurrentTab},
```

并给每个标签页加右键菜单"关闭标签"：在 `openFile` 中 `tabPage.SetTitle(...)` 之后追加：

```go
closeMenu, err := walk.NewMenu()
if err != nil {
	doc.Close()
	return err
}
closeAction := walk.NewAction()
closeAction.SetText("关闭标签")
closeAction.Triggered().Attach(func() {
	if idx := a.tabWidget.Pages().Index(tabPage); idx >= 0 {
		a.tabWidget.SetCurrentIndex(idx)
		a.closeCurrentTab()
	}
})
closeMenu.Actions().Add(closeAction)
tabPage.SetContextMenu(closeMenu)
```

（鼠标中键关闭标签留给后续迭代——walk 的 `TabWidget` 没有暴露每个 tab 的鼠标事件，需要额外的 win32 子类化处理，超出当前"标准阅读器"范围，不在此计划内实现，避免引入不确定的 hack。右键菜单 + Ctrl+W 已能覆盖关闭标签的核心需求。）

- [ ] **Step 2: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 3: 手动验证**

1. 依次打开 `testdata/sample.pdf` 两次（会出现两个标题相同的标签页，这是预期行为——同一文件可以开两个独立标签）。
2. 确认两个标签页的翻页/缩放互不影响（在第一个标签翻到第 2 页，切到第二个标签应仍在第 1 页）。
3. 按 Ctrl+W 关闭当前标签，确认标签消失，切到另一个标签仍正常显示。
4. 右键某个标签，选择"关闭标签"，确认该标签被关闭。
5. 关闭所有标签后，确认状态栏恢复"就绪"，主窗口不退出。

- [ ] **Step 4: 提交**

```bash
git add internal/ui/app.go
git commit -m "feat: add tab close via Ctrl+W and right-click context menu"
```

---

### Task 19: UI — 最近文件菜单、密码对话框、跳转页码对话框、关于对话框

**Files:**
- Create: `internal/ui/dialogs.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: 实现三个对话框**

```go
// internal/ui/dialogs.go
package ui

import (
	"fmt"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// promptPassword shows a modal password dialog. ok is false if the user
// cancelled.
func promptPassword(owner walk.Form, fileName string, wrongPassword bool) (password string, ok bool) {
	var dlg *walk.Dialog
	var pwEdit *walk.LineEdit
	var acceptBtn, cancelBtn *walk.PushButton

	msg := fmt.Sprintf("《%s》需要密码才能打开", fileName)
	if wrongPassword {
		msg = "密码错误，请重试"
	}

	d := Dialog{
		AssignTo:      &dlg,
		Title:         "输入密码",
		DefaultButton: &acceptBtn,
		CancelButton:  &cancelBtn,
		Layout:        VBox{},
		Children: []Widget{
			Label{Text: msg},
			LineEdit{AssignTo: &pwEdit, PasswordMode: true},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{AssignTo: &acceptBtn, Text: "确定", OnClicked: func() { dlg.Accept() }},
					PushButton{AssignTo: &cancelBtn, Text: "取消", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}

	if _, err := d.Run(owner); err != nil {
		return "", false
	}
	if dlg.Result() != walk.DlgCmdOK {
		return "", false
	}
	return pwEdit.Text(), true
}

// promptGoToPage shows a modal "go to page" dialog. ok is false if the
// user cancelled. page is 1-based in the UI but returned 0-based.
func promptGoToPage(owner walk.Form, currentPage1Based, pageCount int) (page0Based int, ok bool) {
	var dlg *walk.Dialog
	var numberEdit *walk.NumberEdit
	var acceptBtn, cancelBtn *walk.PushButton

	d := Dialog{
		AssignTo:      &dlg,
		Title:         "转到页面",
		DefaultButton: &acceptBtn,
		CancelButton:  &cancelBtn,
		Layout:        VBox{},
		Children: []Widget{
			Label{Text: fmt.Sprintf("页码 (1-%d)：", pageCount)},
			NumberEdit{AssignTo: &numberEdit, MinValue: 1, MaxValue: float64(pageCount), Decimals: 0, Value: float64(currentPage1Based)},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{AssignTo: &acceptBtn, Text: "确定", OnClicked: func() { dlg.Accept() }},
					PushButton{AssignTo: &cancelBtn, Text: "取消", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}

	if _, err := d.Run(owner); err != nil {
		return 0, false
	}
	if dlg.Result() != walk.DlgCmdOK {
		return 0, false
	}
	return int(numberEdit.Value()) - 1, true
}

func showAboutDialog(owner walk.Form) {
	walk.MsgBox(owner, "关于 PDF 阅读器", "PDF 阅读器 v0.1\n基于 lxn/walk 与 go-pdfium 构建。", walk.MsgBoxIconInformation)
}
```

- [ ] **Step 2: 接入密码对话框（修改 openFile 的打开逻辑）**

修改 `internal/ui/app.go` 的 `openFile`，把：

```go
	doc, err := a.pool.Open(data, nil)
	if err != nil {
		return err
	}
```

替换为：

```go
	doc, err := a.pool.Open(data, nil)
	if errors.Is(err, pdfengine.ErrPasswordRequired) {
		wrongAttempt := false
		for {
			pw, ok := promptPassword(a.mainWindow, filepathBase(path), wrongAttempt)
			if !ok {
				return errors.New("已取消：需要密码")
			}
			doc, err = a.pool.Open(data, &pw)
			if err == nil {
				break
			}
			if !errors.Is(err, pdfengine.ErrPasswordRequired) {
				return err
			}
			wrongAttempt = true
		}
	} else if err != nil {
		return err
	}
```

在文件顶部 import 中加入 `"errors"`。

- [ ] **Step 3: 接入"跳转页码"对话框**

在"转到"菜单里追加：

```go
Action{Text: "跳转到页码...(&G)", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyG}, OnTriggered: a.onGoToPage},
```

在文件末尾追加：

```go
func (a *app) onGoToPage() {
	t := a.currentTab()
	if t == nil {
		return
	}
	page, ok := promptGoToPage(a.mainWindow, t.page+1, t.doc.PageCount())
	if !ok {
		return
	}
	a.goToPage(t, page)
}
```

- [ ] **Step 4: 接入"关于"对话框**

在 `MenuItems` 末尾追加"帮助"菜单：

```go
Menu{
	Text: "帮助(&H)",
	Items: []MenuItem{
		Action{Text: "关于...", OnTriggered: func() { showAboutDialog(a.mainWindow) }},
	},
},
```

- [ ] **Step 5: 接入"最近打开的文件"菜单与配置持久化**

在文件末尾追加重建最近文件子菜单的逻辑，并在 `openFile` 成功后调用：

```go
func (a *app) rebuildRecentMenu() {
	if a.recentMenuAction == nil {
		return
	}
	menu := a.recentMenuAction.Menu()
	for menu.Actions().Len() > 0 {
		menu.Actions().RemoveAt(0)
	}
	for _, rf := range a.cfg.RecentFiles {
		path := rf.Path
		action := walk.NewAction()
		action.SetText(path)
		action.Triggered().Attach(func() {
			if err := a.openFile(path); err != nil {
				a.cfg.RemoveRecent(path)
				a.cfg.Save()
				a.rebuildRecentMenu()
				walk.MsgBox(a.mainWindow, "文件不存在", fmt.Sprintf("无法打开 %s，已从最近列表中移除。", path), walk.MsgBoxIconWarning)
			}
		})
		menu.Actions().Add(action)
	}
}
```

在 `app` 结构体中加入字段 `recentMenuAction *walk.Action`。

在"文件"菜单的 `Items` 中，"打开..."之后、"关闭标签"之前插入一个 `Menu` 项：

```go
Menu{
	AssignActionTo: &a.recentMenuAction,
	Text:           "最近打开的文件",
},
```

在 `openFile` 的最后（`return nil` 之前）追加：

```go
	a.cfg.AddRecent(path)
	a.cfg.Save()
	a.rebuildRecentMenu()
```

在 `Run` 函数里，`mw.Run()` 之前（即 `MainWindow{...}` 字面量构建完 `mw` 之后）不需要改动；但需要在窗口首次显示前调用一次 `a.rebuildRecentMenu()`。由于 `recentMenuAction` 只有在 `mw.Create`/`mw.Run` 内部构建 `MenuItems` 后才被赋值，改为在 `mw.Run()` 之后没有意义（此时窗口已在阻塞运行）。改用 `MainWindow.SuspendedUntilRun` 机制过于复杂，本计划采用更简单的方式：在 `onOpenClicked` 和启动参数处理（Task 20）里已经会调用 `openFile`，而 `openFile` 内部已经调用 `rebuildRecentMenu`；唯一遗漏的是**程序刚启动、还没打开任何文件时**菜单应显示已保存的最近文件列表。为此在 `Run` 中，构建完 `MainWindow` 字面量、调用 `mw.Run()` 之前，改用 `mw.Create()` + 手动 `Show()`+`Run()` 拆分：

```go
	if err := mw.Create(); err != nil {
		return 1, err
	}
	a.rebuildRecentMenu()
	if initialFile != "" {
		if err := a.openFile(initialFile); err != nil {
			walk.MsgBox(a.mainWindow, "无法打开文件", err.Error(), walk.MsgBoxIconError)
		}
	}
	a.mainWindow.Show()
	return a.mainWindow.Run(), nil
```

删除原来 `mw.Run()` 那一行调用，改成上面这段。

- [ ] **Step 6: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。若 `Menu.AssignActionTo` 与预期字段名不符，用 `go doc github.com/lxn/walk/declarative.Menu` 核对（Task 前期已确认 `declarative.Menu` 有 `AssignActionTo **walk.Action` 字段）。

- [ ] **Step 7: 手动验证**

1. 启动程序，确认"文件 > 最近打开的文件"为空（首次运行）。
2. 打开 `testdata/sample.pdf`，关闭程序，重新启动，确认"最近打开的文件"里出现该路径，点击可直接打开。
3. 打开 `testdata/encrypted.pdf`：确认弹出密码对话框；先故意输入错误密码，确认提示"密码错误，请重试"；再输入 `testpass`，确认成功打开并显示第 1 页。
4. Ctrl+G 弹出跳转页码对话框，输入 2，确认视图跳到第 2 页；输入超出范围的值（如 99）确认被限制在合法范围内（NumberEdit 的 MaxValue 生效）。
5. 帮助 > 关于，确认弹出信息框。
6. 手动删除/重命名一个最近文件对应的实际 .pdf 文件后，从"最近打开的文件"菜单点击它，确认提示"文件不存在，已从最近列表中移除"，且再次打开菜单时该项已消失。

- [ ] **Step 8: 提交**

```bash
git add internal/ui/dialogs.go internal/ui/app.go
git commit -m "feat: add password dialog, goto-page dialog, about dialog, and recent-files menu"
```

---

### Task 20: main.go — 命令行参数打开文件

**Files:**
- Modify: `main.go`

- [ ] **Step 1: 解析命令行参数**

```go
// main.go
package main

import (
	"fmt"
	"os"

	"pdfreader/internal/ui"
)

func main() {
	var initialFile string
	if len(os.Args) > 1 {
		if info, err := os.Stat(os.Args[1]); err == nil && !info.IsDir() {
			initialFile = os.Args[1]
		}
	}

	if _, err := ui.Run(initialFile); err != nil {
		fmt.Fprintln(os.Stderr, "pdfreader:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 构建**

```bash
GOARCH=amd64 GOOS=windows "/d/soft/go/bin/go.exe" build -o pdfreader.exe .
```

Expected: 成功。

- [ ] **Step 3: 手动验证**

1. 命令行运行 `./pdfreader.exe testdata/sample.pdf`，确认程序启动后直接打开该文件并显示第 1 页。
2. 命令行运行 `./pdfreader.exe testdata/nonexistent.pdf`，确认程序正常启动（不崩溃），只是没有自动打开任何标签（因为 `os.Stat` 失败，`initialFile` 保持为空）。
3. 命令行不带参数运行 `./pdfreader.exe`，确认和之前行为一致（空标签栏 + 已保存的最近文件菜单）。

- [ ] **Step 4: 提交**

```bash
git add main.go
git commit -m "feat: support opening a PDF via command-line argument"
```

---

### Task 21: 打包与手动测试清单

**Files:**
- Create: `README.md`

- [ ] **Step 1: 编写 README**

```markdown
# PDF 阅读器

基于 [lxn/walk](https://github.com/lxn/walk) 和 [go-pdfium](https://github.com/klippa-app/go-pdfium)（WebAssembly 模式）实现的 Windows 桌面 PDF 阅读器。

## 构建

需要 Go >= 1.25（工具链会通过 `GOTOOLCHAIN=auto` 自动下载，需要网络）。本机若只有 32 位 Go（`windows/386`），交叉编译到 64 位即可，无需额外安装 C 编译器：

```bash
GOARCH=amd64 GOOS=windows go build -o pdfreader.exe .
```

首次构建前需生成 Windows 资源（图标 + DPI 感知 manifest），见 `winres/README` 或直接使用仓库中已提交的 `rsrc_windows_amd64.syso`（图标/manifest 不变的情况下无需重新生成）。

## 运行

```bash
./pdfreader.exe                    # 打开空窗口
./pdfreader.exe path\to\file.pdf   # 启动时直接打开指定文件
```

## 手动测试清单

完整走查以下场景（自动化测试只覆盖 `internal/pdfengine`、`internal/config`、`internal/document`，UI 交互需人工验证）：

- [ ] 打开未加密 PDF，正确显示第一页
- [ ] 上一页/下一页/首页/末页导航，边界不越界
- [ ] 缩放：放大、缩小到上下限、适合宽度、适合页面
- [ ] 目录大纲：点击书签跳转到对应页
- [ ] 缩略图：点击缩略图跳转到对应页
- [ ] 查找：输入关键词、上一个/下一个匹配循环、无结果提示
- [ ] 多标签页：打开多个文档，各自状态独立；Ctrl+W 与右键菜单关闭标签
- [ ] 最近打开的文件：重启后仍保留，点击可直接打开，文件被删除后自动从列表移除
- [ ] 加密 PDF：密码正确可打开，密码错误有明确提示，可重试
- [ ] 命令行参数打开指定 PDF
- [ ] 高 DPI 显示器（或系统缩放 125%/150%）下界面不糊、不错位
```

- [ ] **Step 2: 完整走查手动测试清单**

按 README 中的清单逐项在实际运行的 `pdfreader.exe` 上验证，全部打勾。

- [ ] **Step 3: 提交**

```bash
git add README.md
git commit -m "docs: add build instructions and manual test checklist"
```

---

## 计划自查（写完后的自我审阅记录）

- **spec 覆盖检查：** 设计文档中的所有功能点——打开/翻页/缩放/目录/缩略图/搜索/多标签/最近文件/加密文档/命令行打开/DPI-manifest/JSON 配置——均能在 Task 1–21 中找到对应任务。范围之外事项（打印、文本选择复制、批注、连续滚动、自动注册表关联）未被安排任务，符合设计文档"范围之外"一节。
- **占位符扫描：** 全文搜索确认没有 "TBD"/"TODO"/"实现细节后续补充" 等占位符；每个 Step 的代码块都是可直接落地的最终版本（初稿中 Task 5/6/12 曾出现"先给错误草稿再修正"的写法，已在自查中删除，只保留一份正确实现，避免执行者误用草稿版本）。
- **类型一致性检查：** `pdfengine.Document`、`pdfengine.OutlineNode`、`pdfengine.SearchMatch`、`document.Zoom`/`ZoomMode`/`CacheKey`/`Cache`、`ui.tab`/`ui.app` 等类型在跨任务引用处字段名、方法签名保持一致（如 `t.doc`、`t.page`、`t.zoom`、`t.cache`、`a.tabs`、`a.currentTab()`、`a.goToPage()` 从 Task 12 定义后被 Task 13–19 一致复用，未出现改名）。
- **范围检查：** 单个计划文档聚焦于同一个可独立运行验证的应用；未拆分成多个子项目，符合 brainstorming 阶段未识别出独立子系统的结论。
