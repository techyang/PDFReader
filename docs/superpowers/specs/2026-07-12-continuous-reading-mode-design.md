# 连续阅读模式设计

**Goal:** 在现有单页阅读模式之外，加一个可切换的"连续阅读模式"——垂直滚动浏览整份文档，只渲染视口内可见的页面，滚动流畅、大文档（几百页）也不会一次性占用大量内存。

**背景：** 原实现计划（`docs/superpowers/plans/2026-07-11-pdf-reader-implementation.md`）的自查记录明确把"连续滚动"列为范围之外。这是一个在 Task 1-21 全部完成后追加的新功能，走独立的 brainstorming → 设计 → 实现流程，不回头改原计划。

---

## 1. 入口与持久化

- "视图"菜单新增一个可勾选（checkable）菜单项"连续阅读模式"，与"打开"菜单里已有的其它 Action 风格一致。
- 状态存进 `config.Config`，新增字段 `ContinuousMode bool`（`json:"continuousMode"`），做法与现有 `SidebarShown`/`SidebarTab` 完全一致：切换时更新 `a.cfg.ContinuousMode` 并 `Save()`。
- 全局生效：所有已打开和之后打开的标签页共用同一个模式，不是每个标签页单独设置。跨重启保留上次的选择。

## 2. 控件结构变化

现状：`pageView`（`walk.CustomWidget`）直接挂在标签页的 `HSplitter` 下，一次只画 `t.page` 这一页，尺寸等于视口。

改造成：`splitter → pageScroll(walk.ScrollView) → pageView(CustomWidget)`。单页模式和连续模式共用这一棵控件树，切换模式不需要重建标签页/不需要 dispose 重来，只需要重新计算 `pageView` 的虚拟尺寸和改变绘制逻辑。

- **单页模式**：`pageView` 尺寸 = 视口大小（和现在完全一致，`pageScroll` 没有可滚动的余量，行为不变）。
- **连续模式**：`pageView` 的虚拟高度 = 所有页在当前缩放下的像素高度总和（含页间间距），宽度取所有页里最宽的一个；这样 `pageScroll` 会出现纵向滚动条。

`tab` 结构体新增字段：`pageScroll *walk.ScrollView`（原有 `pageView` 字段不变）。

## 3. 分层：纯计算逻辑放进 internal/document

延续项目现有的三层结构（`pdfengine`/`document`/`ui`：前两层是可单测的纯逻辑，`ui` 是声明式界面，只负责转发事件），连续模式新增的两块计算也放进 `internal/document`，可以像 `zoom.go`/`cache.go` 一样写单元测试，不必依赖 walk 的 GUI 环境：

```go
// internal/document/continuous.go
package document

// PageLayout is one page's position within the full scrollable content
// of a continuously-scrolled document, at a given zoom level.
type PageLayout struct {
    Top    float64 // px, top offset within the full virtual content
    Width  float64 // px
    Height float64 // px
    DPI    int     // dpi to render this page at (matches pdfengine.RenderPage's dpi param)
}

// LayoutContinuous computes per-page layout for continuous-scroll mode.
// pageSizesPt are each page's (widthPt, heightPt) in document order.
// viewportWidthPx is used for ZoomFitWidth/ZoomPercent scale calculation
// (ZoomFitPage is not meaningful here - callers must not pass it).
// gapPx is the vertical spacing drawn between consecutive pages
// (ui layer passes a constant 8px).
// Returns the per-page layouts and the total virtual content height.
func LayoutContinuous(pageSizesPt [][2]float64, zoom Zoom, viewportWidthPx, gapPx float64) (layouts []PageLayout, totalHeight float64)

// VisiblePages returns the [start,end) index range (into layouts) of
// pages that intersect the vertical range [scrollTop, scrollTop+viewportHeight).
func VisiblePages(layouts []PageLayout, scrollTop, viewportHeight float64) (start, end int)

// MostVisiblePage returns the index into layouts of the page covering
// the largest visible area within [scrollTop, scrollTop+viewportHeight).
// Returns 0 if layouts is empty.
func MostVisiblePage(layouts []PageLayout, scrollTop, viewportHeight float64) int
```

`internal/ui` 里的绘制回调只做：拿 `updateBounds` → 调 `document.VisiblePages` 拿到页码范围 → 对这几页调用 `doc.RenderPage`（复用现有 `t.cache`）→ 按 `PageLayout.Top` 画出来。滚动事件里只做：拿当前滚动偏移 → 调 `document.MostVisiblePage` → 更新 `t.page`/状态栏/侧边栏高亮。真正"哪几页可见""滚动条该停在哪"这些计算全部是可单测的纯函数。

`tab.cache` 目前是 `document.NewCache(5)`。连续模式下视口内同时可见的页数可能超过 5（缩得比较小、或翻页间距较密时），缓存太小会导致"刚滚出视口又滚回来"时反复重新渲染。把 `newTab` 里的容量从 5 调到 16，两种模式都用同一个 cache，单页模式下多出来的容量没有坏处（LRU 淘汰不活跃的旧页），连续模式下能覆盖典型视口的可见页数。

## 4. 当前页判定与导航联动

- 滚动过程中（`ScrollView` 的滚动事件），用当前滚动偏移 + 视口高度调 `document.MostVisiblePage`，更新 `t.page`，驱动状态栏"第 X / Y 页"文字和目录/缩略图侧边栏的高亮跟随。这个方向**不**触发反向跳转滚动，否则会来回打转。
- 反过来，所有"跳转到某页"的入口——上一页/下一页/首页/末页、跳转页码对话框、目录点击、缩略图点击、查找的上一个/下一个匹配——现在全部统一走 `a.goToPage(t, page)`，调用方不用改，只在 `goToPage` 内部按 `a.cfg.ContinuousMode` 分支：连续模式下把 `pageScroll` 滚动到目标页顶部对齐视口顶部；单页模式行为完全不变（设置 `t.page` + `Invalidate`）。

## 5. 缩放交互

- 放大/缩小/适合宽度在连续模式下会重新计算整个布局（`document.LayoutContinuous` 重跑一遍，因为总高度和每页偏移都变了），并把滚动条重新定位到"缩放前那一页在视口里的相对位置"，避免缩放后滚动条位置和内容对不上。
- "适合页面"在连续模式下没有意义（不需要把单页塑造成刚好填满视口），对应菜单项禁用（`Enabled` 绑定一个 condition：`!a.cfg.ContinuousMode`）。如果当前正处于"适合页面"缩放时切换进连续模式，自动降级为"适合宽度"。

## 6. 范围边界（这次明确不做）

- 不做放大后超出视口的水平/任意方向平移查看——这是单页模式本来就有的限制（现在放大超过视口就直接被裁剪），这次不顺带修，只处理垂直方向的连续翻页。
- 不做双页对开视图、不做书籍模式。
- 不做"记住每个文档上次滚动位置"这类持久化；下次打开文档还是从第 1 页开始，跟现在单页模式行为一致。
- 搜索匹配的高亮矩形现状本来就没有画在页面上（`SearchMatch.Rects` 目前只用于定位跳转），这次不新增高亮绘制。

## 7. 测试策略

- `internal/document/continuous.go` 的三个函数（`LayoutContinuous`/`VisiblePages`/`MostVisiblePage`）走 TDD，写 `continuous_test.go`，覆盖：单页文档、多页等宽文档在 FitWidth 下的偏移量、缩放变化后总高度的变化、视口跨越多页时 `VisiblePages` 的边界、视口内两页各占一半时 `MostVisiblePage` 的平局处理。
- `internal/config` 的 `ContinuousMode` 字段走现有的 `TestSaveThenLoad_RoundTrip` 风格测试。
- `internal/ui` 部分和项目里所有其它 UI 任务一样不写自动化测试（walk 没有无头 CI 可用的控件模拟机制），改为手动验证清单，追加到 `README.md` 的手动测试清单里：
  - [ ] 连续模式下滚动，页面依次渲染，无明显卡顿/白屏
  - [ ] 连续模式下滚动，状态栏页码和侧边栏高亮跟随视口内主要可见页更新
  - [ ] 连续模式下点击目录/缩略图/跳转页码/查找匹配，正确滚动到目标页顶部
  - [ ] 连续模式下缩放（放大/缩小/适合宽度），滚动位置视觉上保持在同一处；"适合页面"菜单项禁用
  - [ ] 切换回单页模式，行为与切换前完全一致
  - [ ] 关闭程序重新打开，连续模式的开关状态被记住
