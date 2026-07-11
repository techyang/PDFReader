package pdfengine

import (
	"reflect"
	"testing"

	"github.com/klippa-app/go-pdfium/responses"
)

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

func TestConvertBookmarks(t *testing.T) {
	t.Run("nested tree", func(t *testing.T) {
		in := []responses.GetBookmarksBookmark{
			{
				Title:    "Root",
				DestInfo: &responses.DestInfo{PageIndex: 0},
				Children: []responses.GetBookmarksBookmark{
					{
						Title:    "Child1",
						DestInfo: &responses.DestInfo{PageIndex: 1},
						Children: []responses.GetBookmarksBookmark{
							{
								Title:    "Grandchild",
								DestInfo: &responses.DestInfo{PageIndex: 2},
							},
						},
					},
					{
						Title:    "Child2",
						DestInfo: nil,
					},
				},
			},
		}

		want := []OutlineNode{
			{
				Title:     "Root",
				PageIndex: 0,
				Children: []OutlineNode{
					{
						Title:     "Child1",
						PageIndex: 1,
						Children: []OutlineNode{
							{
								Title:     "Grandchild",
								PageIndex: 2,
								Children:  []OutlineNode{},
							},
						},
					},
					{
						Title:     "Child2",
						PageIndex: -1,
						Children:  []OutlineNode{},
					},
				},
			},
		}

		got := convertBookmarks(in)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("convertBookmarks() =\n%+v\nwant\n%+v", got, want)
		}
	})

	t.Run("no destination defaults to -1", func(t *testing.T) {
		in := []responses.GetBookmarksBookmark{
			{Title: "No Dest", DestInfo: nil},
		}

		got := convertBookmarks(in)
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}
		if got[0].PageIndex != -1 {
			t.Fatalf("got[0].PageIndex = %d, want -1", got[0].PageIndex)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		for name, in := range map[string][]responses.GetBookmarksBookmark{
			"nil":   nil,
			"empty": {},
		} {
			t.Run(name, func(t *testing.T) {
				got := convertBookmarks(in)
				if got == nil {
					t.Fatal("convertBookmarks() = nil, want non-nil empty slice")
				}
				if len(got) != 0 {
					t.Fatalf("len(got) = %d, want 0", len(got))
				}
			})
		}
	})
}
