package ingest

import (
	"archive/zip"
	"bytes"
	"fmt"
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

func TestExtractorReadsPDF(t *testing.T) {
	extractor := NewExtractor(4096)

	text, err := extractor.Extract(bytes.NewReader(minimalPDF("Hello PDF")), "Handbook.pdf", "application/pdf")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(text, "Hello PDF") {
		t.Fatalf("text = %q, want Hello PDF", text)
	}
}

func TestExtractorReadsDOCX(t *testing.T) {
	extractor := NewExtractor(4096)
	docx := zipFixture(map[string]string{
		"word/document.xml": `<?xml version="1.0"?>
			<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
				<w:body><w:p><w:r><w:t>Hello DOCX</w:t></w:r></w:p></w:body>
			</w:document>`,
	})

	text, err := extractor.Extract(bytes.NewReader(docx), "Handbook.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(text, "Hello DOCX") {
		t.Fatalf("text = %q, want Hello DOCX", text)
	}
}

func TestExtractorReadsPPTX(t *testing.T) {
	extractor := NewExtractor(4096)
	pptx := zipFixture(map[string]string{
		"ppt/slides/slide1.xml": `<?xml version="1.0"?>
			<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
				<p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Hello PPTX</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld>
			</p:sld>`,
	})

	text, err := extractor.Extract(bytes.NewReader(pptx), "Deck.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(text, "Hello PPTX") {
		t.Fatalf("text = %q, want Hello PPTX", text)
	}
}

func TestExtractorReadsXLSXSharedStrings(t *testing.T) {
	extractor := NewExtractor(4096)
	xlsx := zipFixture(map[string]string{
		"xl/sharedStrings.xml": `<?xml version="1.0"?>
			<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
				<si><t>Hello XLSX</t></si>
			</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?>
			<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
				<sheetData><row><c t="s"><v>0</v></c><c><v>42</v></c></row></sheetData>
			</worksheet>`,
	})

	text, err := extractor.Extract(bytes.NewReader(xlsx), "Sheet.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(text, "Hello XLSX") || !strings.Contains(text, "42") {
		t.Fatalf("text = %q, want shared string and numeric cell", text)
	}
	if strings.Contains(text, "\n0") || strings.HasPrefix(text, "0") {
		t.Fatalf("text includes shared string index: %q", text)
	}
}

func zipFixture(files map[string]string) []byte {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			panic(err)
		}
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func minimalPDF(text string) []byte {
	stream := fmt.Sprintf("BT /F1 24 Tf 100 700 Td (%s) Tj ET", escapePDFString(text))
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
		fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream),
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for _, object := range objects {
		offsets = append(offsets, buf.Len())
		buf.WriteString(object)
	}
	xrefOffset := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))
	buf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset))
	return buf.Bytes()
}

func escapePDFString(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, "(", `\(`)
	text = strings.ReplaceAll(text, ")", `\)`)
	return text
}
