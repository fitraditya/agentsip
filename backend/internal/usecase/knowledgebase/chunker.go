package knowledgebase

import (
	"strings"
	"unicode"
)

// defaultMaxChars/defaultOverlapChars approximate "~500-800 tokens with
// ~10-15% overlap" using character count rather than a real tokenizer —
// ASSUMPTION: a model-specific token counter would size chunks more
// precisely, but would also mean pulling in a tokenizer per LLM family for a
// provider-agnostic platform. ~4 characters/token for English text is the
// standard rule of thumb this is based on.
const (
	defaultMaxChars     = 2000
	defaultOverlapChars = 200
)

// SimpleChunker splits text on whitespace boundaries into overlapping
// character-length windows. It is the default Chunker — not configurable per
// knowledge base in v1, since a more sophisticated (e.g. semantic or
// sentence-aware) chunker is a swap-in replacement behind the same Chunker
// port, not a reason to add configuration to this one.
type SimpleChunker struct {
	MaxChars     int
	OverlapChars int
}

func NewSimpleChunker() *SimpleChunker {
	return &SimpleChunker{MaxChars: defaultMaxChars, OverlapChars: defaultOverlapChars}
}

func (c *SimpleChunker) Chunk(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	maxChars := c.MaxChars
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	overlap := c.OverlapChars
	if overlap < 0 || overlap >= maxChars {
		overlap = 0
	}

	runes := []rune(text)
	var chunks []string
	start := 0
	for start < len(runes) {
		end := start + maxChars
		if end >= len(runes) {
			end = len(runes)
		} else {
			// Prefer breaking at the last whitespace before end so a chunk
			// doesn't split a word in half; fall back to a hard cut only
			// when the window contains no whitespace at all.
			breakAt := end
			for breakAt > start && !unicode.IsSpace(runes[breakAt]) {
				breakAt--
			}
			if breakAt > start {
				end = breakAt
			}
		}

		if chunk := strings.TrimSpace(string(runes[start:end])); chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}

		next := end - overlap
		if next <= start {
			next = start + 1 // guarantee forward progress regardless of overlap/window size
		}
		// Stepping back by a fixed character count can land mid-word just as
		// easily as the forward cut above can, so the same boundary-snapping
		// applies here: advance to the next whitespace run before starting
		// the next chunk there.
		for next < end && !unicode.IsSpace(runes[next]) {
			next++
		}
		for next < len(runes) && unicode.IsSpace(runes[next]) {
			next++
		}
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return chunks
}
