package pdfengine

import (
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Rect is a bounding box in PDF page point coordinates (origin bottom-left).
type Rect struct {
	Left, Top, Right, Bottom float64
}

// SearchMatch is one match of a search query, with the highlight rects for
// that match on its page.
type SearchMatch struct {
	PageIndex int
	Rects     []Rect
}

// Search searches the whole document for query (case-insensitive) and
// returns one SearchMatch per hit, ordered by page then position.
//
// An empty query returns (nil, nil): pdfium's FPDFText_FindNext never
// advances past a zero-length pattern, so searching for "" would otherwise
// spin forever without ever releasing the page/search handles it holds.
func (d *Document) Search(query string) ([]SearchMatch, error) {
	if query == "" {
		return nil, nil
	}

	var matches []SearchMatch

	for page := 0; page < d.pages; page++ {
		pageMatches, err := d.searchPage(page, query)
		if err != nil {
			return nil, err
		}
		matches = append(matches, pageMatches...)
	}

	return matches, nil
}

func (d *Document) searchPage(page int, query string) ([]SearchMatch, error) {
	loadResp, err := d.instance.FPDFText_LoadPage(&requests.FPDFText_LoadPage{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{Document: d.handle, Index: page},
		},
	})
	if err != nil {
		return nil, err
	}
	textPage := loadResp.TextPage
	defer d.instance.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: textPage})

	startResp, err := d.instance.FPDFText_FindStart(&requests.FPDFText_FindStart{
		TextPage:   textPage,
		Find:       query,
		StartIndex: 0,
	})
	if err != nil {
		return nil, err
	}
	search := startResp.Search
	defer d.instance.FPDFText_FindClose(&requests.FPDFText_FindClose{Search: search})

	var matches []SearchMatch
	for {
		nextResp, err := d.instance.FPDFText_FindNext(&requests.FPDFText_FindNext{Search: search})
		if err != nil {
			return nil, err
		}
		if !nextResp.GotMatch {
			break
		}

		idxResp, err := d.instance.FPDFText_GetSchResultIndex(&requests.FPDFText_GetSchResultIndex{Search: search})
		if err != nil {
			return nil, err
		}
		countResp, err := d.instance.FPDFText_GetSchCount(&requests.FPDFText_GetSchCount{Search: search})
		if err != nil {
			return nil, err
		}

		rects, err := d.matchRects(textPage, idxResp.Index, countResp.Count)
		if err != nil {
			return nil, err
		}

		matches = append(matches, SearchMatch{PageIndex: page, Rects: rects})
	}

	return matches, nil
}

func (d *Document) matchRects(textPage references.FPDF_TEXTPAGE, startIndex, count int) ([]Rect, error) {
	countResp, err := d.instance.FPDFText_CountRects(&requests.FPDFText_CountRects{
		TextPage:   textPage,
		StartIndex: startIndex,
		Count:      count,
	})
	if err != nil {
		return nil, err
	}

	rects := make([]Rect, 0, countResp.Count)
	for i := 0; i < countResp.Count; i++ {
		r, err := d.instance.FPDFText_GetRect(&requests.FPDFText_GetRect{TextPage: textPage, Index: i})
		if err != nil {
			return nil, err
		}
		rects = append(rects, Rect{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Bottom})
	}
	return rects, nil
}
