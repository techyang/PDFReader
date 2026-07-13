// internal/ui/app.go
package ui

import (
	"errors"
	"fmt"
	"os"

	"github.com/klippa-app/go-pdfium"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pdfreader/internal/config"
	"pdfreader/internal/document"
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

	recentMenuAction     *walk.Action
	continuousModeAction *walk.Action
	fitPageAction        *walk.Action

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
		cfg = &config.Config{
			WindowWidth:  config.DefaultWindowWidth,
			WindowHeight: config.DefaultWindowHeight,
		}
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
					Menu{
						AssignActionTo: &a.recentMenuAction,
						Text:           "最近打开的文件",
					},
					Action{Text: "关闭标签(&W)", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyW}, OnTriggered: a.closeCurrentTab},
					Action{Text: "退出(&X)", OnTriggered: func() { a.mainWindow.Close() }},
				},
			},
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
			Menu{
				Text: "转到(&G)",
				Items: []MenuItem{
					Action{Text: "上一页", Shortcut: Shortcut{Key: walk.KeyPrior}, OnTriggered: a.onPrevPage},
					Action{Text: "下一页", Shortcut: Shortcut{Key: walk.KeyNext}, OnTriggered: a.onNextPage},
					Action{Text: "首页", Shortcut: Shortcut{Key: walk.KeyHome}, OnTriggered: a.onFirstPage},
					Action{Text: "末页", Shortcut: Shortcut{Key: walk.KeyEnd}, OnTriggered: a.onLastPage},
					Action{Text: "跳转到页码...(&G)", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyG}, OnTriggered: a.onGoToPage},
				},
			},
			Menu{
				Text: "帮助(&H)",
				Items: []MenuItem{
					Action{Text: "关于...", OnTriggered: func() { showAboutDialog(a.mainWindow) }},
				},
			},
		},
		ToolBar: ToolBar{
			ButtonStyle: ToolBarButtonTextOnly,
			Items: []MenuItem{
				Action{Text: "打开", OnTriggered: a.onOpenClicked},
				Separator{},
				Action{Text: "◀", OnTriggered: a.onPrevPageToolbar},
				Action{Text: "▶", OnTriggered: a.onNextPageToolbar},
			},
		},
		Children: []Widget{
			TabWidget{
				AssignTo: &a.tabWidget,
			},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &a.statusBar, Text: "就绪"},
		},
	}

	if err := mw.Create(); err != nil {
		return 1, err
	}
	a.rebuildRecentMenu()
	a.fitPageAction.SetEnabled(!a.cfg.ContinuousMode)
	if initialFile != "" {
		if err := a.openFile(initialFile); err != nil {
			walk.MsgBox(a.mainWindow, "无法打开文件", err.Error(), walk.MsgBoxIconError)
		}
	}
	a.mainWindow.Show()
	return a.mainWindow.Run(), nil
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

// errFileUnreadable wraps a failure to read path's bytes from disk (the very
// first step of openFile below), so callers - specifically
// rebuildRecentMenu's Triggered handler - can distinguish "the file itself
// is missing/inaccessible on disk" from every other way openFile can fail
// once the bytes were read successfully (password dialog cancelled, wrong
// password given up on, corrupt/unparseable PDF, pdfium pool timeout, ...).
// Only the former should evict the entry from the recent-files list; the
// latter are unrelated to whether the file still exists.
var errFileUnreadable = errors.New("无法读取文件")

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
		return fmt.Errorf("%w: %v", errFileUnreadable, err)
	}

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

	t := newTab(path, doc)

	tabPage, err := walk.NewTabPage()
	if err != nil {
		doc.Close()
		return err
	}
	tabPage.SetTitle(filepathBase(path))

	// closeMenu is tabPage's right-click "关闭标签" context menu. It is
	// built and attached here, immediately after tabPage's creation and
	// well before the many fallible steps below that already dispose
	// tabPage on error (see the comment above the outlinePage/thumbsPage
	// setup for why those need explicit disposal too) - because
	// WindowBase.Dispose() (see walk's window.go) already disposes
	// wb.contextMenu whenever it's set, attaching the menu this early
	// means every later `tabPage.Dispose()` error branch in this
	// function automatically cleans up closeMenu too, with no extra
	// disposal calls needed at each of those sites.
	closeMenu, err := walk.NewMenu()
	if err != nil {
		tabPage.Dispose()
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

	tabPage.SetLayout(walk.NewVBoxLayout())

	splitter, err := walk.NewHSplitter(tabPage)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}

	sidebarComposite, err := walk.NewComposite(splitter)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	sidebarComposite.SetLayout(walk.NewVBoxLayout())

	sidebarTabs, err := walk.NewTabWidget(sidebarComposite)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}

	outlinePage, err := walk.NewTabPage()
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	outlinePage.SetTitle("目录")
	outlinePage.SetLayout(walk.NewVBoxLayout())

	outline, err := doc.Outline()
	if err != nil {
		outline = nil // treat outline errors as "no bookmarks" rather than failing the whole open
	}
	t.outline = outline

	// outlinePage is created via walk.NewTabPage(), which builds a detached,
	// unparented WS_POPUP window (see walk's tabpage.go: InitWindow(tp, nil,
	// ...)). It only becomes a real child of sidebarTabs's native window tree
	// once sidebarTabs.Pages().Add(outlinePage) succeeds (walk's
	// TabWidget.onInsertedPage does win.SetParent(page.hWnd, tw.hWnd) at that
	// point, not at construction). Until then, tabPage.Dispose()'s
	// WM_DESTROY/child-window cascade can't reach outlinePage or anything
	// inside it (e.g. treeView), so every error branch between here and that
	// successful Add call must dispose outlinePage explicitly too.
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
	t.outlineModel = outlineModel
	if err := sidebarTabs.Pages().Add(outlinePage); err != nil {
		outlinePage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}

	thumbsPage, err := walk.NewTabPage()
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	thumbsPage.SetTitle("缩略图")
	thumbsPage.SetLayout(walk.NewVBoxLayout())

	// thumbsPage is subject to the same detached-until-Add-succeeds behavior
	// as outlinePage above (see the comment there), so it too needs an
	// explicit Dispose() on every error branch before sidebarTabs.Pages().Add
	// succeeds - otherwise thumbsScroll and any ImageViews (and their
	// AddDisposable'd bitmaps) already built in a failed buildThumbnails loop
	// would leak, since tabPage.Dispose()'s cascade can't reach them.
	thumbsScroll, err := walk.NewScrollView(thumbsPage)
	if err != nil {
		thumbsPage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}
	thumbsScroll.SetLayout(walk.NewVBoxLayout())

	if err := buildThumbnails(thumbsScroll, doc, func(page int) {
		a.goToPage(t, page)
	}); err != nil {
		thumbsPage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}
	if err := sidebarTabs.Pages().Add(thumbsPage); err != nil {
		thumbsPage.Dispose()
		tabPage.Dispose()
		doc.Close()
		return err
	}

	pageScroll, err := walk.NewScrollView(splitter)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	// Both scrollbars enabled - NOT just "false, true" for horizontal-off.
	// walk.ScrollView's CreateLayoutItem (scrollview.go) only grants a
	// ScrollView GrowableHorz/ShrinkableHorz layout flags when its
	// horizontal scrollbar is enabled; with it disabled, the splitter's
	// layout clamps pageScroll to its tiny "ideal" width instead of the
	// stretch-factor share set below, and every extra pixel piles onto
	// sidebarComposite instead - which is what made the sidebar swallow
	// the entire window. Keeping the horizontal bar enabled also means a
	// page wider than the viewport (zoomed in past 100%, or in continuous
	// mode) can actually be scrolled to instead of just clipping.
	pageScroll.SetScrollbars(true, true)

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

	searchBar, err := a.buildSearchBar(tabPage, t)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	_ = searchBar

	if err := a.tabWidget.Pages().Add(tabPage); err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}

	t.tabPage = tabPage
	t.pageView = pageView

	a.tabs = append(a.tabs, t)
	a.tabWidget.SetCurrentIndex(a.tabWidget.Pages().Len() - 1)

	a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", t.page+1, t.doc.PageCount()))
	pageView.Invalidate()

	a.cfg.AddRecent(path)
	a.cfg.Save()
	a.rebuildRecentMenu()

	// newTab always starts a tab at ZoomFitPage - meaningless in continuous
	// mode (see document.LayoutContinuous's doc comment) and, worse, silently
	// degenerate: ScaleFactor treats a ZoomFitPage call with no viewport
	// height as invalid and falls back to 1.0 (72 DPI), producing a tiny
	// layout. setZoom/onToggleContinuousMode already guard against this for
	// zoom changes on already-open tabs; a freshly opened tab needs the same
	// guard when continuous mode is already on.
	if a.cfg.ContinuousMode && t.zoom.Mode == document.ZoomFitPage {
		t.zoom = document.Zoom{Mode: document.ZoomFitWidth}
	}
	a.applyPageViewMode(t)

	return nil
}

// applyPageViewMode sizes t.pageView to match the app's current reading
// mode (single page vs continuous) and t's current zoom. It's the main
// place that decides pageView's size - called right after a tab is
// created (openFile), whenever its viewport resizes (pageScroll's
// SizeChanged, wired in openFile), whenever its zoom changes (setZoom),
// and whenever the continuous-mode toggle flips for every open tab
// (onToggleContinuousMode). paintTab additionally re-checks and corrects
// the size on every single-page paint as a defensive fallback for the
// case where the very first paint lands before SizeChanged has fired
// even once - both derive the target size the same way and only write
// when it actually differs, so the two can't disagree.
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

func (a *app) paintTab(t *tab, canvas *walk.Canvas, updateBounds walk.Rectangle) error {
	if a.cfg.ContinuousMode {
		return a.paintContinuousTab(t, canvas)
	}

	// pageView's size is normally kept in sync with the viewport by
	// applyPageViewMode, triggered off pageScroll's SizeChanged event. But
	// the very first paint of a freshly created tab - especially one opened
	// via the command-line argument, which happens before the main window
	// is even shown - can land before that event has fired even once
	// (widget creation vs. layout ordering), leaving pageView at whatever
	// size walk.NewCustomWidget gave it by default, not the real viewport.
	//
	// Resizing here and then drawing in the same pass doesn't fully fix
	// that: this paint's canvas is a device context clipped to the OLD
	// (possibly zero) size, established before WM_PAINT reached us, and
	// resizing mid-paint doesn't retroactively widen that clip - the draw
	// call below would silently do nothing. So instead: resize, invalidate
	// to schedule a follow-up paint against the corrected size, and bail
	// out of this one without drawing.
	if viewport := t.pageScroll.ClientBoundsPixels(); viewport.Width > 0 && viewport.Height > 0 {
		if cur := t.pageView.SizePixels(); cur.Width != viewport.Width || cur.Height != viewport.Height {
			t.pageView.SetSizePixels(walk.Size{Width: viewport.Width, Height: viewport.Height})
			t.pageView.Invalidate()
			return nil
		}
	}

	bounds := t.pageView.ClientBounds()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil // not laid out yet - nothing sensible to render
	}

	bmp, err := t.renderCurrentPage(float64(bounds.Width), float64(bounds.Height))
	if err != nil {
		return canvas.DrawText(err.Error(), nil, walk.RGB(200, 0, 0), updateBounds, walk.TextWordbreak)
	}
	defer bmp.Dispose()

	// Center the rendered page within the viewport - it's only exactly
	// as wide/tall as the viewport in ZoomFitWidth/ZoomFitPage; ZoomPercent
	// and any rounding slack in the other two modes leave a margin that
	// otherwise always collects in the bottom-right corner. Clamp to 0:
	// RenderPage's actual pixel output can round up a hair past bounds,
	// and a negative offset would push part of the page off-canvas
	// instead of just trimming the (sub-pixel) margin to nothing.
	size := bmp.Size()
	x := (bounds.Width - size.Width) / 2
	y := (bounds.Height - size.Height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return canvas.DrawImage(bmp, walk.Point{X: x, Y: y})
}

// paintContinuousTab draws every page that intersects the current
// scroll viewport (see document.VisiblePages) at its computed offset,
// and updates t.page/the status bar/the outline selection to track
// whichever page is now most visible.
func (a *app) paintContinuousTab(t *tab, canvas *walk.Canvas) error {
	viewport := t.pageScroll.ClientBoundsPixels()
	if viewport.Width <= 0 || viewport.Height <= 0 {
		return nil // not laid out yet - nothing sensible to render
	}
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

	if a.cfg.ContinuousMode {
		viewport := t.pageScroll.ClientBoundsPixels()
		if err := ensureContinuousLayout(t, float64(viewport.Width)); err == nil && page >= 0 && page < len(t.continuousLayout) {
			setContinuousScrollY(t.pageScroll, t.pageView, t.continuousLayout[page].Top, t.continuousTotalH)
		}
	}
	// Invalidate unconditionally, even on the continuous-mode error path
	// above: paintContinuousTab/paintTab re-run ensureContinuousLayout (or
	// renderCurrentPage) themselves and surface the same error as on-screen
	// red text, instead of leaving the status bar's page number silently
	// out of sync with whatever is (or isn't) actually drawn.
	t.pageView.Invalidate()

	a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", t.page+1, t.doc.PageCount()))
}

// navInputFocused reports whether t's outline tree or search edit currently
// has the keyboard input focus. The page-navigation shortcuts below are
// registered as bare-key accelerators (Home/End/PageUp/PageDown with no
// modifier), so they fire on every keydown regardless of which control has
// focus - including the outline TreeView, whose native SysTreeView32
// control interprets those same keys as "move tree selection", and the
// search bar's LineEdit, whose native Edit control interprets Home/End as
// "move text caret to start/end of line". Without this guard, clicking
// into the sidebar (or the search box) and pressing Home/End would both
// move the tree selection/text caret AND change the current PDF page at
// the same time.
//
// Why this also applies to the search LineEdit: walk subclasses every
// native control's window procedure via SetWindowLongPtr (see walk's
// window.go InitWindow), routing WM_KEYDOWN through
// WindowBase.handleKeyDown first. That function looks up the pressed key
// combo in the package-level shortcut2Action map - which is global, not
// scoped to whichever control has focus - and raises the matching Action's
// OnTriggered before ever falling through to CallWindowProc, which invokes
// the native Edit control's original window procedure (the thing that
// actually moves the caret). So a bare Home/End keystroke while the search
// LineEdit is focused triggers both effects: the menu-registered PDF page
// jump fires first, and the native caret movement happens afterward
// regardless.
//
// This guard is only applied to the keyboard-accelerator path
// (onPrevPage/onNextPage/onFirstPage/onLastPage below, wired to the
// "转到" menu's Shortcut-bearing Actions). It deliberately does NOT apply
// to the toolbar ◀/▶ buttons (see onPrevPageToolbar/onNextPageToolbar) -
// an explicit mouse click is never the redundant-double-fire scenario this
// guard exists for, and applying it there previously caused the toolbar
// buttons to silently do nothing whenever the outline tree had focus
// (including via plain Tab-key cycling, even with an empty outline). The
// same reasoning extends to the search box.
//
// One known, accepted limitation: because walk ties a menu item's
// OnTriggered to the same Action as its Shortcut, there is no way to let an
// explicit mouse click on the "上一页"/"下一页" *menu item* bypass this
// guard while still blocking the keyboard accelerator - both go through the
// same guarded handler. This is considered acceptable since clicking the
// menu item while the tree/search box has focus is a much rarer path than
// the toolbar/Tab-cycling case.
func navInputFocused(t *tab) bool {
	if t == nil {
		return false
	}
	return (t.outlineTree != nil && t.outlineTree.Focused()) ||
		(t.searchEdit != nil && t.searchEdit.Focused())
}

func (a *app) onPrevPage() {
	t := a.currentTab()
	if t == nil || navInputFocused(t) {
		return
	}
	a.goToPage(t, t.page-1)
}

func (a *app) onNextPage() {
	t := a.currentTab()
	if t == nil || navInputFocused(t) {
		return
	}
	a.goToPage(t, t.page+1)
}

func (a *app) onFirstPage() {
	t := a.currentTab()
	if t == nil || navInputFocused(t) {
		return
	}
	a.goToPage(t, 0)
}

func (a *app) onLastPage() {
	t := a.currentTab()
	if t == nil || navInputFocused(t) {
		return
	}
	a.goToPage(t, t.doc.PageCount()-1)
}

// onPrevPageToolbar and onNextPageToolbar back the toolbar's ◀/▶ buttons.
// They intentionally skip the navInputFocused guard applied to
// onPrevPage/onNextPage above - see the comment on navInputFocused for why.
func (a *app) onPrevPageToolbar() {
	t := a.currentTab()
	if t == nil {
		return
	}
	a.goToPage(t, t.page-1)
}

func (a *app) onNextPageToolbar() {
	t := a.currentTab()
	if t == nil {
		return
	}
	a.goToPage(t, t.page+1)
}

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
		if err := ensureContinuousLayout(t, float64(viewport.Width)); err == nil && anchorPage >= 0 && anchorPage < len(t.continuousLayout) {
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

// closeCurrentTab closes the tab at the TabWidget's current index: it
// closes the underlying pdfium document, detaches and destroys the tab's
// entire widget tree, and updates the status bar.
//
// walk's TabWidget.Pages().RemoveAt() (see tabpagelist.go/tabwidget.go in
// the walk module) only detaches the removed TabPage - it calls
// removePage(), which does SetParent(nil) and flips the window style back
// to WS_POPUP, updates TabPageList's bookkeeping, and sends
// TCM_DELETEITEM - but it never calls Dispose() on the page. Without an
// explicit Dispose() here, every tab close would leak the tab's entire
// widget tree: the outline TreeView, the thumbnails ScrollView and all its
// ImageViews with their AddDisposable'd bitmaps (Task 16), the search bar,
// and the page CustomWidget - none of their WM_DESTROY-triggered Dispose()
// cascades (see window.go's WM_DESTROY handler, which calls
// wb.window.Dispose() for whatever native window is being torn down) would
// ever fire, since nothing else calls DestroyWindow on the detached
// TabPage's hWnd.
//
// Dispose() is called AFTER RemoveAt(), not before: RemoveAt() must run
// first so TabWidget's internal state (TabPageList.items, the native tab
// control's item list, current-selection bookkeeping) is fully updated
// while the page is still attached and in a consistent state. Disposing
// the page while it's still an attached, current member of Pages() would
// destroy its native window out from under that still-pending
// bookkeeping. Once detached, disposing it is safe and destroys the whole
// widget subtree regardless of the TabPage's own (now unparented) status,
// since DestroyWindow cascades to real child windows regardless of what
// the top window's own parent is.
func (a *app) closeCurrentTab() {
	idx := a.tabWidget.CurrentIndex()
	if idx < 0 || idx >= len(a.tabs) {
		return
	}

	t := a.tabs[idx]
	tabPage := t.tabPage
	t.doc.Close()

	if err := a.tabWidget.Pages().RemoveAt(idx); err != nil {
		return
	}
	tabPage.Dispose()

	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)

	if len(a.tabs) == 0 {
		a.statusBar.SetText("就绪")
	} else if nt := a.currentTab(); nt != nil {
		a.statusBar.SetText(fmt.Sprintf("第 %d / %d 页", nt.page+1, nt.doc.PageCount()))
	}
}

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

// rebuildRecentMenu clears and repopulates the "最近打开的文件" submenu from
// a.cfg.RecentFiles. It is called once at startup (after the recent-files
// menu action exists but before the window is shown) and again after every
// successful openFile, so the menu always reflects the persisted list.
//
// The clearing loop below (Actions().RemoveAt(0) until empty) does not leak:
// ActionList.RemoveAt calls action.release(), which decrements the Action's
// refCount to 0 (each action here was added exactly once, so refCount was 1)
// and, at that point, deletes the action from walk's global actionsById/
// shortcut2Action maps (see walk's action.go). Menu.onRemovingAction (the
// ActionList's observer callback fired by RemoveAt) also issues the native
// win.RemoveMenu Win32 call against the submenu's HMENU before release()
// runs (see walk's menu.go), so the native menu item is torn down too. None
// of these actions carry a submenu or an image, so release()'s optional
// submenu-Dispose branch never applies. Net effect: every rebuild fully
// tears down the previous batch of actions, both at the Go level and at the
// native Win32 level, before creating the next batch - nothing accumulates
// across repeated opens.
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
			err := a.openFile(path)
			if err == nil {
				return
			}
			if errors.Is(err, errFileUnreadable) {
				// The file itself is gone/inaccessible - evict it from the
				// recent list. Any other openFile failure (password dialog
				// cancelled, wrong password given up on, corrupt PDF, pool
				// timeout, ...) is unrelated to the file's existence and
				// must not evict a perfectly valid recent entry.
				a.cfg.RemoveRecent(path)
				a.cfg.Save()
				a.rebuildRecentMenu()
				walk.MsgBox(a.mainWindow, "文件不存在", fmt.Sprintf("无法打开 %s，已从最近列表中移除。", path), walk.MsgBoxIconWarning)
				return
			}
			walk.MsgBox(a.mainWindow, "无法打开文件", err.Error(), walk.MsgBoxIconError)
		})
		menu.Actions().Add(action)
	}
}

func filepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
