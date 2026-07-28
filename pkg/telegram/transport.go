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

// messageParams builds the send params for r: HTML parse mode, clamped text,
// optional keyboard. This is the ONLY place ModeHTML is set — apps must route
// every send through Send so the invariant holds.
func messageParams(chatID int64, r Reply) *telego.SendMessageParams {
	p := tu.Message(tu.ID(chatID), Clamp(r.Text)).WithParseMode(telego.ModeHTML)
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
