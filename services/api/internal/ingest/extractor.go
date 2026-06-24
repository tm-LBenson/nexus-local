package ingest

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var textExtensions = map[string]bool{
	".txt":  true,
	".md":   true,
	".json": true,
	".html": true,
	".htm":  true,
	".csv":  true,
	".tsv":  true,
	".vtt":  true,
}

type Extractor struct {
	MaxBytes int64
}

func NewExtractor(maxBytes int64) Extractor {
	if maxBytes <= 0 {
		maxBytes = 10 << 20
	}
	return Extractor{MaxBytes: maxBytes}
}

func (e Extractor) Extract(reader io.Reader, name string, contentType string) (string, error) {
	if !looksTextLike(name, contentType) {
		return "", fmt.Errorf("unsupported document type %q", contentType)
	}

	var buf bytes.Buffer
	limit := e.MaxBytes + 1
	written, err := io.CopyN(&buf, reader, limit)
	if err != nil && err != io.EOF {
		return "", err
	}
	if written > e.MaxBytes {
		return "", fmt.Errorf("document exceeds extraction limit of %d bytes", e.MaxBytes)
	}

	content := buf.String()
	if !utf8.ValidString(content) {
		return "", fmt.Errorf("document is not valid UTF-8 text")
	}
	return content, nil
}

func looksTextLike(name string, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case "application/json", "application/xml", "application/xhtml+xml":
		return true
	}
	return textExtensions[strings.ToLower(filepath.Ext(name))]
}
