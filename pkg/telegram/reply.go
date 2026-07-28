package telegram

import "strings"

// Button is a transport-neutral inline-keyboard button. Data is callback data.
type Button struct {
	Text string
	Data string
}

// Reply is a transport-neutral command result: HTML-ready text + optional
// inline-keyboard rows.
type Reply struct {
	Text string
	Rows [][]Button
}

// escaper handles exactly the three characters Telegram's HTML parser treats
// as markup. Order matters: & first.
var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Escape makes dynamic text safe to interpolate into an HTML-mode message.
func Escape(s string) string { return escaper.Replace(s) }
