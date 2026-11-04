// Package textextract turns an uploaded file's raw bytes into clean,
// ingestible plain text for a knowledge base. PDF extraction and
// sanitization both live here deliberately: this is the one place in the
// codebase that ever reads untrusted document formats, so it stays a
// small, framework-agnostic utility with no usecase or HTTP dependencies.
package textextract

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// ExtractPDF reads a PDF's plain text content, in reading order. The
// underlying library is known to panic on malformed input (see its own
// issue tracker), so this recovers and reports that as an ordinary error
// instead of crashing the request.
func ExtractPDF(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("textextract: pdf library panicked: %v", r)
		}
	}()

	reader, openErr := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if openErr != nil {
		return "", fmt.Errorf("textextract: open pdf: %w", openErr)
	}
	plain, plainErr := reader.GetPlainText()
	if plainErr != nil {
		return "", fmt.Errorf("textextract: read pdf text: %w", plainErr)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(plain); err != nil {
		return "", fmt.Errorf("textextract: buffer pdf text: %w", err)
	}
	return buf.String(), nil
}

// Sanitize cleans text pulled out of a PDF (or any uploaded/pasted text)
// before it's chunked and embedded: it strips control characters, a
// leading byte-order mark, and invalid UTF-8 byte sequences; normalizes
// line endings and PDF page-break markers to "\n"; and collapses the long
// runs of blank lines page breaks leave behind. It only removes formatting
// noise — it never alters the actual wording — and it never panics.
//
// This is also where any embedded, non-visible PDF content (form fields,
// attachments, OCG/OCR artifacts the extractor may surface as stray
// control bytes) gets filtered out, since GetPlainText only ever returns
// the document's rendered text stream in the first place — there is no
// separate attachment or embedded-file content to strip here.
func Sanitize(text string) string {
	text = strings.ToValidUTF8(text, "")
	text = strings.TrimPrefix(text, "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\f", "\n")

	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}

	lines := strings.Split(b.String(), "\n")
	out := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			blankRun++
			if blankRun > 1 {
				continue
			}
		} else {
			blankRun = 0
		}
		out = append(out, trimmed)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
