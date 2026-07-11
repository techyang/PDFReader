package pdfengine

import (
	"errors"
	"os"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return data
}

func TestOpenDocument_Success(t *testing.T) {
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

	if got := doc.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}
}

func TestOpenDocument_PasswordRequired(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	_, err = pool.Open(readTestdata(t, "encrypted.pdf"), nil)
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("Open() err = %v, want ErrPasswordRequired", err)
	}
}

func TestOpenDocument_WrongPassword(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	wrong := "wrongpass"
	_, err = pool.Open(readTestdata(t, "encrypted.pdf"), &wrong)
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("Open() err = %v, want ErrPasswordRequired", err)
	}
}

func TestOpenDocument_CorrectPassword(t *testing.T) {
	pool, err := NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	pw := "testpass"
	doc, err := pool.Open(readTestdata(t, "encrypted.pdf"), &pw)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	if got := doc.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}
}
