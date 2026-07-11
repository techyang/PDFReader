package pdfengine

import "testing"

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
