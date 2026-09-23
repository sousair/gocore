package telegram

import "strings"

// MaxLen is Telegram's per-message character ceiling.
const MaxLen = 4096

const ellipsis = "…"

// openTags scans HTML produced by this package and returns the tag names left
// open at the end of s, in the order they were opened. The package only ever
// emits the simple, well-formed Telegram tag set, so a light scan suffices —
// it does not need to be a general HTML parser.
// ponytail: assumes our own renderer's output (no attributes except <a href>,
// no self-closing tags); not a general-purpose HTML sanitizer.
func openTags(s string) []string {
	var stack []string
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			break
		}
		tag := s[i+1 : i+end]
		i += end
		if tag == "" {
			continue
		}
		if tag[0] == '/' {
			stack = closeOpenTag(stack, tag[1:])
			continue
		}
		stack = append(stack, tagName(tag))
	}
	return stack
}

// closeOpenTag removes the innermost occurrence of name from stack. An
// unmatched close is a no-op — malformed input just carries the stray open
// state forward.
func closeOpenTag(stack []string, name string) []string {
	for j := len(stack) - 1; j >= 0; j-- {
		if stack[j] == name {
			return append(stack[:j], stack[j+1:]...)
		}
	}
	return stack
}

// tagName returns the element name, stripping any attributes (e.g. `a href="..."`).
func tagName(tag string) string {
	if sp := strings.IndexByte(tag, ' '); sp >= 0 {
		return tag[:sp]
	}
	return tag
}

// closeTags returns the closing tags for open in LIFO order.
func closeTags(open []string) string {
	var b strings.Builder
	for i := len(open) - 1; i >= 0; i-- {
		b.WriteString("</")
		b.WriteString(open[i])
		b.WriteString(">")
	}
	return b.String()
}

// safeCut returns the largest prefix of s not longer than limit bytes that ends
// on a rune boundary and not inside a `<...>` tag.
func safeCut(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	// back off out of any partial tag
	if lt := strings.LastIndexByte(s[:cut], '<'); lt >= 0 {
		if gt := strings.IndexByte(s[lt:], '>'); gt < 0 || lt+gt >= cut {
			cut = lt
		}
	}
	// back off to a rune boundary
	for cut > 0 && !utf8ValidBoundary(s, cut) {
		cut--
	}
	return s[:cut]
}

func utf8ValidBoundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	return s[i]&0xC0 != 0x80 // not a UTF-8 continuation byte
}

// Clamp truncates already-rendered HTML to fit MaxLen, appending an ellipsis
// and closing any tags left open by the cut. Balanced input under the limit is
// returned unchanged.
func Clamp(s string) string {
	if len(s) <= MaxLen {
		return s
	}
	// Reserve room for the ellipsis; closing tags are added after and are small.
	body := safeCut(s, MaxLen-len(ellipsis))
	open := openTags(body)
	tail := ellipsis + closeTags(open)
	// If closing tags pushed us over, cut a little more and retry once.
	for len(body)+len(tail) > MaxLen && len(body) > 0 {
		body = safeCut(body, len(body)-1)
		open = openTags(body)
		tail = ellipsis + closeTags(open)
	}
	return body + tail
}

// Split breaks s into MaxLen-sized chunks on newline/space boundaries, each
// independently tag-balanced via Clamp's closing logic.
func Split(s string) []string {
	if len(s) <= MaxLen {
		return []string{s}
	}
	var out []string
	for len(s) > MaxLen {
		chunk, open := shrinkChunkToFit(safeCut(s, splitCutPoint(s)))
		out = append(out, chunk+closeTags(open))
		s = reopenCarriedTags(open, chunk, strings.TrimLeft(s[len(chunk):], " \n"))
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// splitCutPoint returns the byte offset within s[:MaxLen] to cut on: the last
// newline if there is one, else the last space, else MaxLen itself.
func splitCutPoint(s string) int {
	cut := MaxLen
	if nl := strings.LastIndexByte(s[:cut], '\n'); nl > 0 {
		return nl
	}
	if sp := strings.LastIndexByte(s[:cut], ' '); sp > 0 {
		return sp
	}
	return cut
}

// shrinkChunkToFit trims chunk until it plus its still-open tags' closing
// tags fits MaxLen, returning the trimmed chunk and its open tags.
func shrinkChunkToFit(chunk string) (string, []string) {
	open := openTags(chunk)
	for len(chunk)+len(closeTags(open)) > MaxLen && len(chunk) > 0 {
		// Decrement by max(1, chunk/8) to make progress with nested tags.
		decr := len(chunk) / 8
		if decr == 0 {
			decr = 1
		}
		chunk = safeCut(chunk, len(chunk)-decr)
		open = openTags(chunk)
	}
	return chunk, open
}

// reopenCarriedTags prepends open's tags to s so a chunk boundary mid-nesting
// doesn't lose the enclosing element in the next chunk. Guards forward
// progress: only reopens if the prefix is shorter than the chunk it follows,
// so s can't grow across iterations (bounds pathological deep-nesting inputs
// against non-termination).
func reopenCarriedTags(open []string, chunk, s string) string {
	if len(open) == 0 {
		return s
	}
	var b strings.Builder
	for _, t := range open {
		b.WriteString("<")
		b.WriteString(t)
		b.WriteString(">")
	}
	reopen := b.String()
	if len(reopen) < len(chunk) {
		return reopen + s
	}
	return s
}
