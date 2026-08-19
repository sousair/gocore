// Package telegram provides safe construction and sending of Telegram
// HTML-mode messages for gocore consumers.
//
// The rules this package enforces (the cross-repo Telegram standard):
//
//  1. Never interpolate dynamic text into an HTML-mode message without Escape.
//  2. Never send LLM/markdown prose without HTML() conversion.
//  3. Always clamp/split outbound text to the 4096-character limit.
//
// Usage:
//
//   - Templated replies: wrap each dynamic field in Escape.
//     Reply{Text: "✏️ " + Escape(merchant) + " → " + Escape(category)}
//   - LLM/markdown prose: convert the whole answer with HTML.
//     Reply{Text: HTML(advisorAnswer)}
//   - Send replies through Send (clamps to one message for cards/edits) or
//     SendLong (splits across messages for model-authored text, since Clamp
//     silently discards the tail). Both route through htmlMessage, the single
//     site where telego.ModeHTML is set.
//
// Telegram HTML mode supports only <b> <i> <u> <s> <code> <pre> <a>
// <blockquote>. HTML() emits only these and degrades everything else to text.
package telegram
