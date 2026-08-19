package telegram

import (
	"context"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Keyboard builds a telego inline keyboard from neutral button rows.
// Returns nil when there are no rows (telego treats nil as "no keyboard").
func Keyboard(rows [][]Button) *telego.InlineKeyboardMarkup {
	if len(rows) == 0 {
		return nil
	}
	kbRows := make([][]telego.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		kbRow := make([]telego.InlineKeyboardButton, 0, len(row))
		for _, btn := range row {
			kbRow = append(kbRow, tu.InlineKeyboardButton(btn.Text).WithCallbackData(btn.Data))
		}
		kbRows = append(kbRows, kbRow)
	}
	return tu.InlineKeyboard(kbRows...)
}

// htmlMessage is the single site where ModeHTML is applied. Both send paths
// go through it, so the "one place sets the parse mode" invariant survives
// having more than one kind of send.
func htmlMessage(chatID int64, text string) *telego.SendMessageParams {
	return tu.Message(tu.ID(chatID), text).WithParseMode(telego.ModeHTML)
}

// messageParams builds the send params for r: HTML parse mode, clamped text,
// optional keyboard. It routes through htmlMessage to maintain the single site
// for ModeHTML.
func messageParams(chatID int64, r Reply) *telego.SendMessageParams {
	p := htmlMessage(chatID, Clamp(r.Text))
	if kb := Keyboard(r.Rows); kb != nil {
		p = p.WithReplyMarkup(kb)
	}
	return p
}

// Send delivers r to chatID as an HTML-mode message.
func Send(ctx context.Context, bot *telego.Bot, chatID int64, r Reply) error {
	_, err := bot.SendMessage(ctx, messageParams(chatID, r))
	return err
}

// longMessageParams renders r as one message per Split chunk. The keyboard
// goes on the last chunk only, so buttons sit under the end of the answer
// rather than in the middle of it.
func longMessageParams(chatID int64, r Reply) []*telego.SendMessageParams {
	chunks := Split(r.Text)
	out := make([]*telego.SendMessageParams, 0, len(chunks))
	for i, c := range chunks {
		p := htmlMessage(chatID, c)
		if i == len(chunks)-1 {
			if kb := Keyboard(r.Rows); kb != nil {
				p = p.WithReplyMarkup(kb)
			}
		}
		out = append(out, p)
	}
	return out
}

// SendLong delivers r without truncating it: text over MaxLen is split across
// consecutive messages rather than clamped. Prefer this over Send for anything
// a model authored — Send's Clamp silently discards the tail.
func SendLong(ctx context.Context, bot *telego.Bot, chatID int64, r Reply) error {
	for _, p := range longMessageParams(chatID, r) {
		if _, err := bot.SendMessage(ctx, p); err != nil {
			return err
		}
	}
	return nil
}
