// internal/ui/printdialog.go
package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

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

// printDialogState holds the state showPrintDialog's closures need to
// share across the dialog's lifetime, beyond what's already captured
// directly as local variables (the widget pointers, and a itself via
// the enclosing method's receiver) - just the file list and the
// printer-dependent state that isn't tied to any single widget.
type printDialogState struct {
	items        []*printItem
	printerNames []string
	paperSizes   []print.PaperSize
	baseDevMode  *win.DEVMODE // set by onProperties; nil until the user opens "属性" at least once
}

// showPrintDialog opens the print dialog. initial, if non-nil, is
// pre-added to the file list as a borrowed (not owned) item - this is
// how openFile's currently-active tab gets pre-filled (see app.go's
// onPrint). onRun is called once the user clicks "打印" with the final
// item list and settings; it's a callback rather than this function
// doing the printing itself so a later task can wire in the progress
// dialog without touching this file again.
func (a *app) showPrintDialog(initial *printItem, onRun func(items []print.Item, settings print.Settings)) {
	st := &printDialogState{}
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
	// would only ever run once nobody can see it happen. Everything
	// computed here only runs ONCE, at dialog-open time; all *subsequent*
	// updates (add/remove a file, switch printers, ...) happen from the
	// event handlers further down, which run while the dialog is live and
	// their AssignTo'd widgets already exist - those are fine to call
	// .SetModel()/.SetText() on directly.
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
							{AssignTo: &allPagesBtn, Text: "所有页", OnClicked: func() {
								rangeEdit.SetEnabled(false)
								saveRangeForSelection()
							}},
							{AssignTo: &customBtn, Text: "选择页", OnClicked: func() {
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
							{AssignTo: &portraitBtn, Text: "纵向"},
							{AssignTo: &landscapeBtn, Text: "横向"},
						},
					},
					Label{Text: "页面大小调整："},
					RadioButtonGroup{
						Buttons: []RadioButton{
							{AssignTo: &fitBtn, Text: "适合页面"},
							{AssignTo: &actualBtn, Text: "实际大小"},
							{AssignTo: &percentBtn, Text: "页面缩放"},
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

	// Deviation from the plan's literal source: declarative.RadioButton
	// has no Checked field (unlike CheckBox), so initial radio selection
	// can't be set through the Dialog{} literal above - trying to
	// compiles as "unknown field Checked in struct literal of type
	// declarative.RadioButton". d.Create() builds every widget
	// (including AssignTo'ing all the *walk.RadioButton pointers)
	// synchronously and returns before the modal message loop starts
	// (Dialog.Run(owner) in the walk source is just Create(owner) then
	// (*AssignTo).Run()), so the initial checked state is applied here
	// instead, once the widgets exist, via SetChecked - then dlg.Run()
	// takes over to block until Accept()/Cancel()/window close.
	if err := d.Create(a.mainWindow); err != nil {
		walk.MsgBox(a.mainWindow, "打印", fmt.Sprintf("无法创建打印对话框：%v", err), walk.MsgBoxIconError)
		for _, it := range st.items {
			if it.ownsDoc {
				it.doc.Close()
			}
		}
		return
	}
	allPagesBtn.SetChecked(initialAllPages)
	customBtn.SetChecked(!initialAllPages)
	portraitBtn.SetChecked(a.cfg.LastOrientation != "landscape")
	landscapeBtn.SetChecked(a.cfg.LastOrientation == "landscape")
	fitBtn.SetChecked(a.cfg.LastScaleMode != "actual" && a.cfg.LastScaleMode != "percent")
	actualBtn.SetChecked(a.cfg.LastScaleMode == "actual")
	percentBtn.SetChecked(a.cfg.LastScaleMode == "percent")

	dlg.Run()

	// Any item the dialog itself opened (ownsDoc) that's still in the
	// list when the dialog closes (cancelled, or closed after a
	// successful print) must be closed here - a later task's RunJob call
	// only reads from these Documents, it never takes ownership of
	// closing them.
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
