package telegram

import (
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension"
	extastnode "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(goldmark.WithExtensions(extast.Strikethrough))

// attrEscaper escapes the characters unsafe inside a double-quoted HTML
// attribute value: Escape's three plus the quote that would otherwise close it.
var attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// escapeAttr makes dynamic text safe to interpolate into a double-quoted HTML
// attribute (e.g. href). Unlike Escape, it also escapes '"'.
func escapeAttr(s string) string { return attrEscaper.Replace(s) }

// HTML converts LLM/markdown prose into Telegram-safe HTML. It emits only the
// tags Telegram's HTML parser accepts and escapes every text node; unsupported
// blocks degrade to text (headings→bold, bullets→"• ", ordered lists keep their
// number, images are dropped).
func HTML(src string) string {
	source := []byte(src)
	root := md.Parser().Parse(text.NewReader(source))
	var b strings.Builder
	renderNode(&b, root, source, listCtx{})
	return strings.TrimRight(b.String(), "\n")
}

// listCtx threads the enclosing list's ordering into item rendering.
type listCtx struct {
	ordered bool
	index   int
}

func renderChildren(b *strings.Builder, n ast.Node, src []byte, lc listCtx) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		renderNode(b, c, src, lc)
	}
}

func renderNode(b *strings.Builder, n ast.Node, src []byte, lc listCtx) {
	switch node := n.(type) {
	case *ast.Document:
		renderChildren(b, n, src, lc)
	case *ast.Paragraph:
		renderChildren(b, n, src, lc)
		b.WriteString("\n")
	case *ast.TextBlock:
		renderChildren(b, n, src, lc)
	case *ast.Heading:
		b.WriteString("<b>")
		renderChildren(b, n, src, lc)
		b.WriteString("</b>\n")
	case *ast.Text:
		b.WriteString(Escape(string(node.Segment.Value(src))))
		if node.SoftLineBreak() || node.HardLineBreak() {
			b.WriteString("\n")
		}
	case *ast.String:
		b.WriteString(Escape(string(node.Value)))
	case *ast.Emphasis:
		tag := "i"
		if node.Level == 2 {
			tag = "b"
		}
		b.WriteString("<" + tag + ">")
		renderChildren(b, n, src, lc)
		b.WriteString("</" + tag + ">")
	case *extastnode.Strikethrough:
		b.WriteString("<s>")
		renderChildren(b, n, src, lc)
		b.WriteString("</s>")
	case *ast.CodeSpan:
		b.WriteString("<code>")
		b.WriteString(Escape(inlineText(n, src)))
		b.WriteString("</code>")
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		b.WriteString("<pre>")
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			b.WriteString(Escape(string(seg.Value(src))))
		}
		b.WriteString("</pre>\n")
	case *ast.Link:
		b.WriteString(`<a href="`)
		b.WriteString(escapeAttr(string(node.Destination)))
		b.WriteString(`">`)
		renderChildren(b, n, src, lc)
		b.WriteString("</a>")
	case *ast.AutoLink:
		url := string(node.URL(src))
		b.WriteString(`<a href="`)
		b.WriteString(escapeAttr(url))
		b.WriteString(`">`)
		b.WriteString(Escape(url))
		b.WriteString("</a>")
	case *ast.Image:
		renderChildren(b, n, src, lc)
	case *ast.List:
		i := node.Start
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			renderNode(b, c, src, listCtx{ordered: node.IsOrdered(), index: i})
			i++
		}
	case *ast.ListItem:
		if lc.ordered {
			b.WriteString(strconv.Itoa(lc.index))
			b.WriteString(". ")
		} else {
			b.WriteString("• ")
		}
		before := b.Len()
		renderChildren(b, n, src, lc)
		if b.Len() == before || b.String()[b.Len()-1] != '\n' {
			b.WriteString("\n")
		}
	case *ast.Blockquote:
		b.WriteString("<blockquote>")
		renderChildren(b, n, src, lc)
		trimTrailingNewline(b)
		b.WriteString("</blockquote>\n")
	case *ast.ThematicBreak:
		b.WriteString("\n")
	case *ast.RawHTML:
		for i := 0; i < node.Segments.Len(); i++ {
			seg := node.Segments.At(i)
			b.WriteString(Escape(string(seg.Value(src))))
		}
	case *ast.HTMLBlock:
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			b.WriteString(Escape(string(seg.Value(src))))
		}
		if node.HasClosure() {
			closure := node.ClosureLine
			b.WriteString(Escape(string(closure.Value(src))))
		}
		b.WriteString("\n")
	default:
		renderChildren(b, n, src, lc)
	}
}

// inlineText collects the literal text content of n's children, escaping
// nothing itself — callers escape the result. Unlike relying on *ast.Text
// alone, it recurses through any child kind so no inline content (e.g. a
// *ast.String, or nested inlines) is silently dropped.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			b.WriteString(string(t.Segment.Value(src)))
		case *ast.String:
			b.WriteString(string(t.Value))
		default:
			b.WriteString(inlineText(c, src))
		}
	}
	return b.String()
}

func trimTrailingNewline(b *strings.Builder) {
	s := b.String()
	if strings.HasSuffix(s, "\n") {
		b.Reset()
		b.WriteString(s[:len(s)-1])
	}
}
