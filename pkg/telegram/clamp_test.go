package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClampShortUnchanged(t *testing.T) {
	if got := Clamp("hello <b>world</b>"); got != "hello <b>world</b>" {
		t.Fatalf("short input changed: %q", got)
	}
}

func TestClampNeverCutsInsideTag(t *testing.T) {
	// Long run of bold text; the cut point lands inside the content.
	s := "<b>" + strings.Repeat("x", MaxLen) + "</b>"
	got := Clamp(s)
	if len(got) > MaxLen {
		t.Fatalf("clamp exceeded MaxLen: %d", len(got))
	}
	if strings.Count(got, "<b>") != strings.Count(got, "</b>") {
		t.Fatalf("unbalanced <b> after clamp: %q", got[len(got)-20:])
	}
	if !strings.HasSuffix(got, "…</b>") {
		t.Fatalf("expected ellipsis inside closed bold, got tail: %q", got[len(got)-10:])
	}
}

func TestSplitFitsReturnsSingle(t *testing.T) {
	if got := Split("small"); len(got) != 1 || got[0] != "small" {
		t.Fatalf("Split(small) = %v", got)
	}
}

func TestSplitChunksUnderLimit(t *testing.T) {
	line := strings.Repeat("a", 1000) + "\n"
	s := strings.Repeat(line, 6) // ~6006 bytes, forces >1 chunk
	got := Split(s)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	for i, c := range got {
		if len(c) > MaxLen {
			t.Fatalf("chunk %d over MaxLen: %d", i, len(c))
		}
	}
}

// TestSplitTagStraddlesBoundaryNoWhitespace covers the case where there is no
// space/newline anywhere near MaxLen to cut on, and a <a href="..."> tag
// straddles the MaxLen boundary itself. Regression test for the bug where
// Split called safeCut(s[:cut], cut) — a no-op, since len(s[:cut]) == cut —
// so the tag/rune backoff never ran and chunks could end mid-tag.
func TestSplitTagStraddlesBoundaryNoWhitespace(t *testing.T) {
	prefix := strings.Repeat("x", MaxLen-10)
	tag := `<a href="http://example.com/verylongpath">linktext</a>`
	s := prefix + tag + strings.Repeat("y", 2000)

	got := Split(s)
	for i, c := range got {
		if len(c) > MaxLen {
			t.Fatalf("chunk %d exceeds MaxLen: %d bytes", i, len(c))
		}
		if !utf8.ValidString(c) {
			t.Fatalf("chunk %d is not valid UTF-8: %q", i, c)
		}
		if lt := strings.LastIndexByte(c, '<'); lt >= 0 && !strings.Contains(c[lt:], ">") {
			end := c[lt:]
			if len(end) > 20 {
				end = end[:20]
			}
			t.Fatalf("chunk %d ends inside a tag: %q", i, end)
		}
	}
}

func TestSplitDeeplyNestedTagsStayUnderLimit(t *testing.T) {
	// Build deeply nested tags with enough content to force multiple chunks.
	// Closing tags have meaningful size; verifies they don't overflow MaxLen.
	var b strings.Builder
	nesting := 50 // ~150 bytes of opening, ~200 bytes of closing tags
	for i := 0; i < nesting; i++ {
		b.WriteString("<b>")
	}
	// Add enough content to force multiple chunks when closing tags are included.
	// chunk + closeTags must fit in MaxLen.
	b.WriteString(strings.Repeat("x", 5000))
	for i := 0; i < nesting; i++ {
		b.WriteString("</b>")
	}
	s := b.String()

	got := Split(s)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks for deeply nested tags, got %d", len(got))
	}
	for i, c := range got {
		if len(c) > MaxLen {
			t.Fatalf("chunk %d over MaxLen: %d bytes", i, len(c))
		}
		// Verify tag balance in each chunk
		if strings.Count(c, "<b>") != strings.Count(c, "</b>") {
			t.Fatalf("chunk %d has unbalanced <b> tags: %d open, %d close",
				i, strings.Count(c, "<b>"), strings.Count(c, "</b>"))
		}
	}
}
