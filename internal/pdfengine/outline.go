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
