package telegram

import (
	"strings"
	"testing"

	"github.com/mymmrac/telego"
)

func TestKeyboardNilWhenEmpty(t *testing.T) {
	if Keyboard(nil) != nil {
		t.Fatal("expected nil keyboard for no rows")
	}
}

func TestKeyboardMapsButtons(t *testing.T) {
	kb := Keyboard([][]Button{{{Text: "Yes", Data: "y"}, {Text: "No", Data: "n"}}})
	if kb == nil || len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("unexpected keyboard shape: %+v", kb)
	}
	if kb.InlineKeyboard[0][0].Text != "Yes" || kb.InlineKeyboard[0][0].CallbackData != "y" {
		t.Fatalf("button not mapped: %+v", kb.InlineKeyboard[0][0])
	}
}

func TestMessageParamsSetsHTMLAndClamps(t *testing.T) {
	long := strings.Repeat("a", MaxLen+100)
	p := messageParams(42, Reply{Text: long})
	if p.ParseMode != telego.ModeHTML {
		t.Fatalf("parse mode = %q, want HTML", p.ParseMode)
	}
	if len([]rune(p.Text)) > MaxLen {
		t.Fatalf("text not clamped: %d runes", len([]rune(p.Text)))
	}
}

func TestMessageParamsAttachesKeyboard(t *testing.T) {
	p := messageParams(1, Reply{Text: "hi", Rows: [][]Button{{{Text: "Go", Data: "g"}}}})
	if p.ReplyMarkup == nil {
		t.Fatal("expected keyboard attached")
	}
}

func TestLongMessageParamsSplitsAndKeepsEveryChunkUnderMaxLen(t *testing.T) {
	// Spaces give Split real word boundaries to cut on.
	long := strings.Repeat("palavra ", (MaxLen/8)*3)
	ps := longMessageParams(7, Reply{Text: long})
	if len(ps) < 2 {
		t.Fatalf("expected several messages, got %d", len(ps))
	}
	for i, p := range ps {
		if len([]rune(p.Text)) > MaxLen {
			t.Errorf("chunk %d is %d runes, over MaxLen %d", i, len([]rune(p.Text)), MaxLen)
		}
		if p.ParseMode != telego.ModeHTML {
			t.Errorf("chunk %d parse mode = %q, want HTML", i, p.ParseMode)
		}
	}
}

func TestLongMessageParamsDoesNotTruncate(t *testing.T) {
	long := strings.Repeat("palavra ", (MaxLen/8)*3)
	ps := longMessageParams(7, Reply{Text: long})
	texts := make([]string, len(ps))
	for i, p := range ps {
		texts[i] = p.Text
	}
	// Split consumes the whitespace it cuts on, so the chunks are rejoined
	// with a single space — one separator per boundary, exactly what was
	// removed. Concatenating with no separator instead would merge the last
	// word of each chunk into the first word of the next and report a loss
	// that never happened.
	joined := strings.Join(texts, " ")
	if strings.Contains(joined, "…") {
		t.Error("split output contains the clamp ellipsis — content was truncated, not split")
	}
	if got, want := len(strings.Fields(joined)), len(strings.Fields(long)); got != want {
		t.Errorf("word count = %d, want %d — content lost across chunks", got, want)
	}
}

func TestLongMessageParamsAttachesKeyboardToLastChunkOnly(t *testing.T) {
	long := strings.Repeat("palavra ", (MaxLen/8)*3)
	ps := longMessageParams(7, Reply{Text: long, Rows: [][]Button{{{Text: "Go", Data: "g"}}}})
	for i := 0; i < len(ps)-1; i++ {
		if ps[i].ReplyMarkup != nil {
			t.Errorf("chunk %d carries a keyboard; only the last may", i)
		}
	}
	if ps[len(ps)-1].ReplyMarkup == nil {
		t.Error("last chunk has no keyboard")
	}
}

func TestLongMessageParamsSingleMessageMatchesMessageParams(t *testing.T) {
	r := Reply{Text: "curto", Rows: [][]Button{{{Text: "Go", Data: "g"}}}}
	ps := longMessageParams(7, r)
	if len(ps) != 1 {
		t.Fatalf("expected 1 message, got %d", len(ps))
	}
	if ps[0].Text != messageParams(7, r).Text || ps[0].ReplyMarkup == nil {
		t.Error("short reply should be identical to the messageParams path")
	}
}
