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
			t.searchMatches = nil
			t.searchIndex = 0
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
