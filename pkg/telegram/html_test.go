package telegram

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
)

func TestHTML(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bold", "**hi**", "<b>hi</b>"},
		{"italic underscore", "_hi_", "<i>hi</i>"},
		{"italic star", "*hi*", "<i>hi</i>"},
		{"strike", "~~hi~~", "<s>hi</s>"},
		{"inline code", "`x`", "<code>x</code>"},
		{"link", "[t](http://e.com)", `<a href="http://e.com">t</a>`},
		{"heading to bold", "# Title", "<b>Title</b>"},
		{"unordered bullet", "- item", "• item"},
		{"ordered keeps number", "1. first\n2. second", "1. first\n2. second"},
		{"escape stray angle", "keep EC < 1.2", "keep EC &lt; 1.2"},
		{"escape ampersand", "AT&T", "AT&amp;T"},
		{"code with angle escaped", "`a<b>`", "<code>a&lt;b&gt;</code>"},
		{"bold inside item keeps number", "1. **Watering:** dry out", "1. <b>Watering:</b> dry out"},
		{"plain paragraph", "just text", "just text"},
		{"autolink", "<http://e.com>", `<a href="http://e.com">http://e.com</a>`},
		{"autolink href attr-escaped, text plain-escaped",
			`<http://e.com/x?a=1&b="y>`,
			`<a href="http://e.com/x?a=1&amp;b=&quot;y">http://e.com/x?a=1&amp;b="y</a>`},
		{"literal html in prose escaped, not emitted", "<div>hi</div>", "&lt;div&gt;hi&lt;/div&gt;"},
		{"fenced code block escaped", "```\na<b>&c\n```\n", "<pre>a&lt;b&gt;&amp;c\n</pre>"},
		{"blockquote", "> quote text\n", "<blockquote>quote text</blockquote>"},
		{"image dropped keeps alt", "![alt](http://img.example/x.png)", "alt"},
		{"link destination quote escaped", `[t](<http://e.com/"y>)`, `<a href="http://e.com/&quot;y">t</a>`},
	}
	for _, c := range cases {
		if got := HTML(c.in); got != c.want {
			t.Errorf("%s: HTML(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestCodeSpanRendersNonTextChildren guards against the CodeSpan renderer
// only handling *ast.Text children. Built directly against the AST since
// goldmark's default parser never itself produces a non-Text CodeSpan child;
// the risk is a future extension (or a manually built tree) doing so.
func TestCodeSpanRendersNonTextChildren(t *testing.T) {
	span := ast.NewCodeSpan()
	span.AppendChild(span, ast.NewString([]byte("a")))
	span.AppendChild(span, ast.NewString([]byte("<b>")))
	var b strings.Builder
	renderNode(&b, span, nil, listCtx{})
	if got, want := b.String(), "<code>a&lt;b&gt;</code>"; got != want {
		t.Fatalf("renderNode(CodeSpan) = %q, want %q", got, want)
	}
}

func TestHTMLBalancedOnScreenshotFixture(t *testing.T) {
	in := "**Before you reset, ensure you address the core issues:**\n\n" +
		"1. **Watering:** Allow the top inch to dry out.\n" +
		"2. **Drainage:** Ensure pots have drainage holes.\n"
	got := HTML(in)
	if want := "<b>Before you reset, ensure you address the core issues:</b>"; !strings.Contains(got, want) {
		t.Errorf("bold header missing:\n%s", got)
	}
	if strings.Contains(got, "**") {
		t.Errorf("literal ** survived:\n%s", got)
	}
	if !strings.Contains(got, "1. <b>Watering:</b>") || !strings.Contains(got, "2. <b>Drainage:</b>") {
		t.Errorf("ordered numbers/bold not preserved:\n%s", got)
	}
}
