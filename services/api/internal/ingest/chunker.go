package ingest

import (
	"strings"
	"unicode/utf8"
)

type Chunk struct {
	ID    string
	Index int
	Text  string
}

type Chunker struct {
	MaxRunes     int
	OverlapRunes int
}

func NewChunker(maxRunes int, overlapRunes int) Chunker {
	if maxRunes <= 0 {
		maxRunes = 1200
	}
	if overlapRunes < 0 {
		overlapRunes = 0
	}
	if overlapRunes >= maxRunes {
		overlapRunes = maxRunes / 5
	}
	return Chunker{
		MaxRunes:     maxRunes,
		OverlapRunes: overlapRunes,
	}
}

func (c Chunker) Chunk(text string) []Chunk {
	cleaned := strings.Join(strings.Fields(text), " ")
	if cleaned == "" {
		return nil
	}
	if utf8.RuneCountInString(cleaned) <= c.MaxRunes {
		return []Chunk{{ID: "chunk_0000", Index: 0, Text: cleaned}}
	}

	runes := []rune(cleaned)
	chunks := make([]Chunk, 0, len(runes)/c.MaxRunes+1)
	start := 0
	for start < len(runes) {
		end := start + c.MaxRunes
		if end > len(runes) {
			end = len(runes)
		} else {
			end = boundaryBefore(runes, end, start)
		}
		chunkText := strings.TrimSpace(string(runes[start:end]))
		if chunkText != "" {
			chunks = append(chunks, Chunk{
				ID:    chunkID(len(chunks)),
				Index: len(chunks),
				Text:  chunkText,
			})
		}
		if end == len(runes) {
			break
		}
		start = end - c.OverlapRunes
		if start < 0 {
			start = 0
		}
		if start >= end {
			start = end
		}
	}
	return chunks
}

func boundaryBefore(runes []rune, desired int, floor int) int {
	for i := desired; i > floor; i-- {
		if runes[i-1] == ' ' || runes[i-1] == '\n' || runes[i-1] == '\t' {
			return i
		}
	}
	return desired
}

func chunkID(index int) string {
	const digits = "0123456789"
	buf := []byte("chunk_0000")
	value := index
	for i := len(buf) - 1; i >= len("chunk_"); i-- {
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf)
}
