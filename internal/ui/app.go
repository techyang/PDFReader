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

	treeView, err := walk.NewTreeView(outlinePage)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	if err := treeView.SetModel(newOutlineModel(outline)); err != nil {
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

	thumbsScroll, err := walk.NewScrollView(thumbsPage)
	if err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	thumbsScroll.SetLayout(walk.NewVBoxLayout())

	if err := buildThumbnails(thumbsScroll, doc, func(page int) {
		a.goToPage(t, page)
	}); err != nil {
		tabPage.Dispose()
		doc.Close()
		return err
	}
	if err := sidebarTabs.Pages().Add(thumbsPage); err != nil {
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

// outlineFocused reports whether t's outline tree currently has the
// keyboard input focus. The page-navigation shortcuts below are registered
// as bare-key accelerators (Home/End/PageUp/PageDown with no modifier), so
// they fire on every keydown regardless of which control has focus -
// including the outline TreeView, whose native SysTreeView32 control
// interprets those same keys as "move tree selection". Without this guard,
// clicking into the sidebar and pressing Home/End would both move the tree
// selection AND change the current PDF page at the same time.
//
// This guard is only applied to the keyboard-accelerator path
// (onPrevPage/onNextPage/onFirstPage/onLastPage below, wired to the
// "转到" menu's Shortcut-bearing Actions). It deliberately does NOT apply
// to the toolbar ◀/▶ buttons (see onPrevPageToolbar/onNextPageToolbar) -
// an explicit mouse click is never the redundant-double-fire scenario this
// guard exists for, and applying it there previously caused the toolbar
// buttons to silently do nothing whenever the outline tree had focus
// (including via plain Tab-key cycling, even with an empty outline).
//
// One known, accepted limitation: because walk ties a menu item's
// OnTriggered to the same Action as its Shortcut, there is no way to let an
// explicit mouse click on the "上一页"/"下一页" *menu item* bypass this
// guard while still blocking the keyboard accelerator - both go through the
// same guarded handler. This is considered acceptable since clicking the
// menu item while the tree has focus is a much rarer path than the
// toolbar/Tab-cycling case.
func outlineFocused(t *tab) bool {
	return t != nil && t.outlineTree != nil && t.outlineTree.Focused()
}

func (a *app) onPrevPage() {
	t := a.currentTab()
	if t == nil || outlineFocused(t) {
		return
	}
	a.goToPage(t, t.page-1)
}

func (a *app) onNextPage() {
	t := a.currentTab()
	if t == nil || outlineFocused(t) {
		return
	}
	a.goToPage(t, t.page+1)
}

func (a *app) onFirstPage() {
	t := a.currentTab()
	if t == nil || outlineFocused(t) {
		return
	}
	a.goToPage(t, 0)
}

func (a *app) onLastPage() {
	t := a.currentTab()
	if t == nil || outlineFocused(t) {
		return
	}
	a.goToPage(t, t.doc.PageCount()-1)
}

// onPrevPageToolbar and onNextPageToolbar back the toolbar's ◀/▶ buttons.
// They intentionally skip the outlineFocused guard applied to
// onPrevPage/onNextPage above - see the comment on outlineFocused for why.
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

func filepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
