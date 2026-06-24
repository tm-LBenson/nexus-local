package ingest

import (
	"strings"
	"testing"
)

func TestExtractorReadsTextLikeFile(t *testing.T) {
	extractor := NewExtractor(1024)

	text, err := extractor.Extract(strings.NewReader("# Handbook"), "Handbook.md", "text/markdown")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if text != "# Handbook" {
		t.Fatalf("text = %q", text)
	}
}

func TestExtractorRejectsUnsupportedType(t *testing.T) {
	extractor := NewExtractor(1024)

	_, err := extractor.Extract(strings.NewReader("fake"), "photo.png", "image/png")
	if err == nil {
		t.Fatal("err = nil, want unsupported type")
	}
}

func TestExtractorRejectsOversizedText(t *testing.T) {
	extractor := NewExtractor(4)

	_, err := extractor.Extract(strings.NewReader("hello"), "note.txt", "text/plain")
	if err == nil {
		t.Fatal("err = nil, want size limit error")
	}
}
