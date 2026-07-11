package pdfengine

import (
	"github.com/klippa-app/go-pdfium"
	pdfiumerrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// ErrPasswordRequired is returned by Pool.Open when the document is
// encrypted and no password (or the wrong password) was supplied.
var ErrPasswordRequired = pdfiumerrors.ErrPassword

// Pool wraps a go-pdfium WebAssembly instance pool.
type Pool struct {
	pool pdfium.Pool
}

// NewPool creates a new pdfium instance pool running the PDFium
// WebAssembly build via wazero (no cgo required).
func NewPool() (*Pool, error) {
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle:  1,
		MaxIdle:  2,
		MaxTotal: 4,
	})
	if err != nil {
		return nil, err
	}
	return &Pool{pool: pool}, nil
}

// Close shuts down the pool and all its instances.
func (p *Pool) Close() error {
	return p.pool.Close()
}
