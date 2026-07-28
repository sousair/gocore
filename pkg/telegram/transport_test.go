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
