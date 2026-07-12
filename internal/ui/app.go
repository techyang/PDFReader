// internal/ui/app.go
package ui

import (
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
					Action{Text: "适合页面", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.Key0}, OnTriggered: a.onFitPage},
					Action{Text: "查找", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyF}, OnTriggered: a.onToggleSearch},
				},
			},
			Menu{
				Text: "转到(&G)",
				Items: []MenuItem{
					Action{Text: "上一页", Shortcut: Shortcut{Key: walk.KeyPrior}, OnTriggered: a.onPrevPage},
					Action{Text: "下一页", Shortcut: Shortcut{Key: walk.KeyNext}, OnTriggered: a.onNextPage},
					Action{Text: "首页", Shortcut: Shortcut{Key: walk.KeyHome}, OnTriggered: a.onFirstPage},
					Action{Text: "末页", Shortcut: Shortcut{Key: walk.KeyEnd}, OnTriggered: a.onLastPage},
				},
			},
		},
		ToolBar: ToolBar{
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

	return mw.Run()
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
	if err := treeView.SetModel(newOutlineModel(outline)); err != nil {
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

	pageView, err := walk.NewCustomWidget(splitter, 0, func(canvas *walk.Canvas, updateBounds walk.Rectangle) error {
		return a.paintTab(t, canvas, updateBounds)
	})
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	pageView.SetClearsBackground(true)

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

func filepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
