package textextract_test

import (
	"os"
	"strings"
	"testing"

	"github.com/fitraditya/agentsip/internal/pkg/textextract"
)

func TestExtractPDF_ReadsRealPDFText(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	text, err := textextract.ExtractPDF(data)
	if err != nil {
		t.Fatalf("ExtractPDF: %v", err)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("expected non-empty extracted text")
	}
}

func TestExtractPDF_RejectsGarbageWithoutPanicking(t *testing.T) {
	if _, err := textextract.ExtractPDF([]byte("not a pdf")); err == nil {
		t.Fatal("expected an error for non-PDF bytes")
	}
}

func TestSanitize_NormalizesLineEndingsAndPageBreaks(t *testing.T) {
	in := "line one\r\nline two\rpage one\fpage two"
	want := "line one\nline two\npage one\npage two"
	if got := textextract.Sanitize(in); got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}

func TestSanitize_StripsBOMAndControlCharacters(t *testing.T) {
	in := "\uFEFFhello\x00 wor\x07ld"
	want := "hello world"
	if got := textextract.Sanitize(in); got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}

func TestSanitize_CollapsesLongRunsOfBlankLines(t *testing.T) {
	in := "para one\n\n\n\n\npara two"
	want := "para one\n\npara two"
	if got := textextract.Sanitize(in); got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}

func TestSanitize_TrimsTrailingWhitespacePerLineAndOverall(t *testing.T) {
	in := "  \nline with trailing spaces   \n  \n"
	want := "line with trailing spaces"
	if got := textextract.Sanitize(in); got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}

func TestSanitize_KeepsOrdinaryTextUnchanged(t *testing.T) {
	in := "Hello, world! This is a normal sentence — with punctuation."
	if got := textextract.Sanitize(in); got != in {
		t.Fatalf("Sanitize() = %q, want unchanged %q", got, in)
	}
}

func TestSanitize_NeverPanicsOnInvalidUTF8(t *testing.T) {
	in := string([]byte{0xff, 0xfe, 'h', 'i', 0x80})
	got := textextract.Sanitize(in)
	if !strings.Contains(got, "hi") {
		t.Fatalf("Sanitize() = %q, want it to retain the valid %q substring", got, "hi")
	}
}
