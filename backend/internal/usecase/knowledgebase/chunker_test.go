package knowledgebase_test

import (
	"strings"
	"testing"

	"github.com/fitraditya/agentsip/internal/usecase/knowledgebase"
)

func TestSimpleChunker_EmptyText(t *testing.T) {
	c := knowledgebase.NewSimpleChunker()
	if got := c.Chunk("   "); got != nil {
		t.Fatalf("Chunk(whitespace) = %v, want nil", got)
	}
}

func TestSimpleChunker_ShortTextIsOneChunk(t *testing.T) {
	c := knowledgebase.NewSimpleChunker()
	got := c.Chunk("hello world")
	if len(got) != 1 || got[0] != "hello world" {
		t.Fatalf("Chunk(short text) = %v, want one chunk", got)
	}
}

func TestSimpleChunker_SplitsLongTextWithoutCuttingWords(t *testing.T) {
	c := &knowledgebase.SimpleChunker{MaxChars: 50, OverlapChars: 10}

	// Build text as repeated words so it's easy to check no chunk ends or
	// starts mid-word.
	text := strings.Repeat("alpha beta gamma delta epsilon ", 10)
	chunks := c.Chunk(text)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for long text, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if chunk == "" {
			t.Fatalf("got an empty chunk in %v", chunks)
		}
		for word := range strings.FieldsSeq(chunk) {
			switch word {
			case "alpha", "beta", "gamma", "delta", "epsilon":
			default:
				t.Fatalf("chunk contains a split/garbled word: %q (chunk: %q)", word, chunk)
			}
		}
	}
}

func TestSimpleChunker_NoWhitespaceStillTerminates(t *testing.T) {
	c := &knowledgebase.SimpleChunker{MaxChars: 10, OverlapChars: 5}
	text := strings.Repeat("x", 100) // no whitespace anywhere to break on

	chunks := c.Chunk(text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	var rebuilt strings.Builder
	for _, c := range chunks {
		rebuilt.WriteString(c)
	}
	if !strings.Contains(rebuilt.String(), text[:10]) {
		t.Fatalf("chunks don't appear to cover the original text: %v", chunks)
	}
}
