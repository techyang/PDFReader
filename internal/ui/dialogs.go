// internal/ui/dialogs.go
package ui

import (
	"fmt"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// promptPassword shows a modal password dialog. ok is false if the user
// cancelled.
//
// Each call builds and runs a brand new walk.Dialog (via the declarative
// Dialog{}.Run() below). This is safe to call repeatedly in a retry loop:
// walk.Dialog.Run() blocks in FormBase.mainLoop() until the dialog's native
// window handle is destroyed, which only happens once Accept()/Cancel()
// (or the window's own close button) sends WM_CLOSE and FormBase's
// WM_CLOSE handler calls close() -> Dispose() -> DestroyWindow (see
// walk's form.go/dialog.go/window.go). DestroyWindow synchronously
// cascades WM_DESTROY to every native child window (Label, LineEdit,
// Composite, PushButtons), each of which is subclassed to Dispose() itself
// in response - the same cascade this codebase already relies on for
// tabPage.Dispose() elsewhere. So by the time Run() returns here, the
// entire dialog widget tree from that call has already been torn down;
// nothing accumulates across repeated wrong-password attempts.
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
