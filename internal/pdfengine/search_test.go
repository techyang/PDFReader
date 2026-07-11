package pdfengine

import "testing"

func TestSearch_MatchesOnBothPages(t *testing.T) {
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

	matches, err := doc.Search("world")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	if matches[0].PageIndex != 0 {
		t.Fatalf("matches[0].PageIndex = %d, want 0", matches[0].PageIndex)
	}
	if matches[1].PageIndex != 1 {
		t.Fatalf("matches[1].PageIndex = %d, want 1", matches[1].PageIndex)
	}
	if len(matches[0].Rects) == 0 {
		t.Fatal("matches[0].Rects is empty, want at least one highlight rect")
	}
}

func TestSearch_NoMatches(t *testing.T) {
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

	matches, err := doc.Search("nonexistentterm")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("len(matches) = %d, want 0", len(matches))
	}
}

// TestSearch_EmptyQuery guards against a regression of a real hang: pdfium's
// FPDFText_FindNext never advances past a zero-length pattern, so without an
// explicit guard, Search("") spins forever and never releases its page/
// search handles (which, given the pool's small MaxTotal, would eventually
// exhaust the pool and hang every other pool.Open call). This test relies on
// `go test`'s own -timeout to fail loudly if that guard is ever removed.
func TestSearch_EmptyQuery(t *testing.T) {
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

	matches, err := doc.Search("")
	if err != nil {
		t.Fatalf("Search(\"\") err = %v, want nil", err)
	}
	if matches != nil {
		t.Fatalf("Search(\"\") matches = %v, want nil", matches)
	}
}
