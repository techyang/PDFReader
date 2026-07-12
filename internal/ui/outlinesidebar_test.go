// internal/ui/outlinesidebar_test.go
package ui

import (
	"testing"

	"pdfreader/internal/pdfengine"
)

func TestOutlineModel(t *testing.T) {
	nodes := []pdfengine.OutlineNode{
		{
			Title:     "Chapter 1",
			PageIndex: 0,
			Children: []pdfengine.OutlineNode{
				{Title: "Section 1.1", PageIndex: 1},
				{Title: "Section 1.2", PageIndex: 2},
			},
		},
		{
			Title:     "Chapter 2",
			PageIndex: 3,
		},
	}

	m := newOutlineModel(nodes)

	if got := m.RootCount(); got != 2 {
		t.Fatalf("RootCount() = %d, want 2", got)
	}

	root0, ok := m.RootAt(0).(*outlineItem)
	if !ok {
		t.Fatalf("RootAt(0) is not *outlineItem")
	}
	if got := root0.Text(); got != "Chapter 1" {
		t.Errorf("root0.Text() = %q, want %q", got, "Chapter 1")
	}
	if got := root0.ChildCount(); got != 2 {
		t.Fatalf("root0.ChildCount() = %d, want 2", got)
	}
	if root0.Parent() != nil {
		t.Errorf("root0.Parent() = %v, want nil", root0.Parent())
	}

	child0, ok := root0.ChildAt(0).(*outlineItem)
	if !ok {
		t.Fatalf("root0.ChildAt(0) is not *outlineItem")
	}
	if got := child0.Text(); got != "Section 1.1" {
		t.Errorf("child0.Text() = %q, want %q", got, "Section 1.1")
	}
	if got := child0.node.PageIndex; got != 1 {
		t.Errorf("child0.node.PageIndex = %d, want 1", got)
	}
	if child0.ChildCount() != 0 {
		t.Errorf("child0.ChildCount() = %d, want 0", child0.ChildCount())
	}
	if parent, ok := child0.Parent().(*outlineItem); !ok || parent != root0 {
		t.Errorf("child0.Parent() = %v, want root0", child0.Parent())
	}

	root1, ok := m.RootAt(1).(*outlineItem)
	if !ok {
		t.Fatalf("RootAt(1) is not *outlineItem")
	}
	if got := root1.Text(); got != "Chapter 2" {
		t.Errorf("root1.Text() = %q, want %q", got, "Chapter 2")
	}
	if root1.ChildCount() != 0 {
		t.Errorf("root1.ChildCount() = %d, want 0", root1.ChildCount())
	}
}

func TestOutlineModelEmpty(t *testing.T) {
	m := newOutlineModel(nil)
	if got := m.RootCount(); got != 0 {
		t.Fatalf("RootCount() = %d, want 0", got)
	}
}

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
