package ingest

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
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

const (
	kindText = "text"
	kindPDF  = "pdf"
	kindDOCX = "docx"
	kindPPTX = "pptx"
	kindXLSX = "xlsx"
)

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
	data, err := readLimited(reader, e.MaxBytes)
	if err != nil {
		return "", err
	}

	switch detectKind(name, contentType) {
	case kindText:
		return extractText(data)
	case kindPDF:
		return extractPDF(data)
	case kindDOCX:
		return extractDOCX(data)
	case kindPPTX:
		return extractPPTX(data)
	case kindXLSX:
		return extractXLSX(data)
	default:
		return "", fmt.Errorf("unsupported document type %q", contentType)
	}
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	var buf bytes.Buffer
	limit := maxBytes + 1
	written, err := io.CopyN(&buf, reader, limit)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if written > maxBytes {
		return nil, fmt.Errorf("document exceeds extraction limit of %d bytes", maxBytes)
	}
	return buf.Bytes(), nil
}

func extractText(data []byte) (string, error) {
	content := string(data)
	if !utf8.ValidString(content) {
		return "", fmt.Errorf("document is not valid UTF-8 text")
	}
	return content, nil
}

func extractPDF(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("read pdf: %w", err)
	}
	plainText, err := pdfReader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract pdf text: %w", err)
	}
	extracted, err := io.ReadAll(plainText)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(extracted))
	if text == "" {
		return "", fmt.Errorf("pdf produced no text")
	}
	return text, nil
}

func extractDOCX(data []byte) (string, error) {
	return extractOpenXML(data, openXMLSpec{
		files: []string{
			"word/document.xml",
			"word/footnotes.xml",
			"word/endnotes.xml",
		},
		prefixes: []string{
			"word/header",
			"word/footer",
		},
		textElements:  map[string]bool{"t": true},
		breakElements: map[string]bool{"p": true, "br": true, "tab": true},
	})
}

func extractPPTX(data []byte) (string, error) {
	return extractOpenXML(data, openXMLSpec{
		prefixes:      []string{"ppt/slides/slide"},
		textElements:  map[string]bool{"t": true},
		breakElements: map[string]bool{"p": true, "br": true},
	})
}

func extractXLSX(data []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("read xlsx archive: %w", err)
	}

	sharedStrings, err := readSharedStrings(archive.File)
	if err != nil {
		return "", err
	}

	sheets := matchingOpenXMLFiles(archive.File, openXMLSpec{prefixes: []string{"xl/worksheets/sheet"}})
	var builder strings.Builder
	for _, sheet := range sheets {
		text, err := extractWorksheetText(sheet, sharedStrings)
		if err != nil {
			return "", err
		}
		appendBlock(&builder, text)
	}
	text := strings.TrimSpace(builder.String())
	if text == "" && len(sharedStrings) > 0 {
		text = strings.Join(sharedStrings, "\n")
	}
	if text == "" {
		return "", fmt.Errorf("xlsx document produced no text")
	}
	return text, nil
}

type openXMLSpec struct {
	files         []string
	prefixes      []string
	textElements  map[string]bool
	breakElements map[string]bool
}

func extractOpenXML(data []byte, spec openXMLSpec) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("read openxml archive: %w", err)
	}

	files := matchingOpenXMLFiles(archive.File, spec)
	var builder strings.Builder
	for _, file := range files {
		text, err := extractXMLText(file, spec)
		if err != nil {
			return "", err
		}
		appendBlock(&builder, text)
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return "", fmt.Errorf("openxml document produced no text")
	}
	return text, nil
}

func matchingOpenXMLFiles(files []*zip.File, spec openXMLSpec) []*zip.File {
	exact := map[string]bool{}
	for _, name := range spec.files {
		exact[name] = true
	}

	matched := make([]*zip.File, 0)
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if exact[name] {
			matched = append(matched, file)
			continue
		}
		for _, prefix := range spec.prefixes {
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".xml") {
				matched = append(matched, file)
				break
			}
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})
	return matched
}

func extractXMLText(file *zip.File, spec openXMLSpec) (string, error) {
	body, err := file.Open()
	if err != nil {
		return "", err
	}
	defer body.Close()

	decoder := xml.NewDecoder(body)
	var builder strings.Builder
	captureDepth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", file.Name, err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			if spec.textElements[token.Name.Local] {
				captureDepth++
			}
			if spec.breakElements[token.Name.Local] {
				appendBreak(&builder)
			}
		case xml.EndElement:
			if spec.textElements[token.Name.Local] && captureDepth > 0 {
				captureDepth--
			}
		case xml.CharData:
			if captureDepth > 0 {
				appendInline(&builder, string(token))
			}
		}
	}
	return strings.TrimSpace(builder.String()), nil
}

func readSharedStrings(files []*zip.File) ([]string, error) {
	for _, file := range files {
		if filepath.ToSlash(file.Name) != "xl/sharedStrings.xml" {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer body.Close()

		decoder := xml.NewDecoder(body)
		shared := make([]string, 0)
		inString := false
		captureDepth := 0
		var current strings.Builder
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("parse shared strings: %w", err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				if token.Name.Local == "si" {
					inString = true
					current.Reset()
				}
				if inString && token.Name.Local == "t" {
					captureDepth++
				}
			case xml.EndElement:
				if inString && token.Name.Local == "t" && captureDepth > 0 {
					captureDepth--
				}
				if token.Name.Local == "si" {
					shared = append(shared, strings.TrimSpace(current.String()))
					inString = false
				}
			case xml.CharData:
				if captureDepth > 0 {
					appendInline(&current, string(token))
				}
			}
		}
		return shared, nil
	}
	return nil, nil
}

func extractWorksheetText(file *zip.File, sharedStrings []string) (string, error) {
	body, err := file.Open()
	if err != nil {
		return "", err
	}
	defer body.Close()

	decoder := xml.NewDecoder(body)
	var builder strings.Builder
	var value strings.Builder
	inCell := false
	cellType := ""
	captureDepth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", file.Name, err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "row":
				appendBreak(&builder)
			case "c":
				inCell = true
				cellType = attrValue(token.Attr, "t")
				value.Reset()
			case "v", "t":
				if inCell {
					captureDepth++
				}
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "v", "t":
				if captureDepth > 0 {
					captureDepth--
				}
			case "c":
				appendInline(&builder, resolveCellValue(strings.TrimSpace(value.String()), cellType, sharedStrings))
				inCell = false
				cellType = ""
				value.Reset()
			}
		case xml.CharData:
			if captureDepth > 0 {
				appendInline(&value, string(token))
			}
		}
	}
	return strings.TrimSpace(builder.String()), nil
}

func resolveCellValue(value string, cellType string, sharedStrings []string) string {
	if cellType != "s" {
		return value
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index >= len(sharedStrings) {
		return value
	}
	return sharedStrings[index]
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, attr := range attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func appendInline(builder *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if builder.Len() > 0 {
		last := builder.String()[builder.Len()-1]
		if last != ' ' && last != '\n' {
			builder.WriteByte(' ')
		}
	}
	builder.WriteString(text)
}

func appendBreak(builder *strings.Builder) {
	if builder.Len() == 0 {
		return
	}
	if !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteByte('\n')
	}
}

func appendBlock(builder *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteByte('\n')
	}
	builder.WriteString(text)
	builder.WriteByte('\n')
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

func detectKind(name string, contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	extension := strings.ToLower(filepath.Ext(name))

	switch {
	case contentType == "application/pdf" || extension == ".pdf":
		return kindPDF
	case contentType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || extension == ".docx":
		return kindDOCX
	case contentType == "application/vnd.openxmlformats-officedocument.presentationml.presentation" || extension == ".pptx":
		return kindPPTX
	case contentType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || extension == ".xlsx":
		return kindXLSX
	case looksTextLike(name, contentType):
		return kindText
	default:
		return ""
	}
}
