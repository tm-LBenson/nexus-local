package ingest

import (
	"strings"
	"testing"
)

func TestChunkerSplitsTextWithOverlap(t *testing.T) {
	chunker := NewChunker(20, 5)

	chunks := chunker.Chunk("alpha beta gamma delta epsilon zeta eta theta")
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	if chunks[0].ID != "chunk_0000" || chunks[1].ID != "chunk_0001" {
		t.Fatalf("chunk ids = %q/%q", chunks[0].ID, chunks[1].ID)
	}
	if !strings.Contains(chunks[1].Text, "delta") {
		t.Fatalf("second chunk = %q, want overlap content", chunks[1].Text)
	}
}

func TestChunkerNormalizesWhitespace(t *testing.T) {
	chunker := NewChunker(100, 0)

	chunks := chunker.Chunk("alpha\n\nbeta\tgamma")
	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(chunks))
	}
	if chunks[0].Text != "alpha beta gamma" {
		t.Fatalf("text = %q", chunks[0].Text)
	}
}
