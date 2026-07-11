package pdfengine

import (
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Document is an open PDF document. It owns a pdfium instance from the
// pool for its entire lifetime; call Close when done with it.
type Document struct {
	instance pdfium.Pdfium
	handle   references.FPDF_DOCUMENT
	pages    int
}

// Open opens a PDF document from raw bytes. password may be nil for
// unencrypted documents. If the document is encrypted and password is
// nil or incorrect, Open returns an error that satisfies
// errors.Is(err, ErrPasswordRequired).
func (p *Pool) Open(data []byte, password *string) (*Document, error) {
	instance, err := p.pool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, err
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		File:     &data,
		Password: password,
	})
	if err != nil {
		instance.Close()
		return nil, err
	}

	pageCount, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		instance.Close()
		return nil, err
	}

	return &Document{
		instance: instance,
		handle:   doc.Document,
		pages:    pageCount.PageCount,
	}, nil
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return d.pages
}

// Close releases the document and returns the pdfium instance to the pool.
func (d *Document) Close() error {
	if _, err := d.instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.handle}); err != nil {
		d.instance.Close()
		return err
	}
	return d.instance.Close()
}
