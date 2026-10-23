package telegram

import (
	"context"
	"log/slog"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/sousair/gocore/pkg/telemetry"
)

// Bot is the subset of *telego.Bot the thinking placeholder drives.
type Bot interface {
	SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error)
	EditMessageText(ctx context.Context, params *telego.EditMessageTextParams) (*telego.Message, error)
	DeleteMessage(ctx context.Context, params *telego.DeleteMessageParams) error
}

var _ Bot = (*telego.Bot)(nil)

// ponytail: one failed edit (e.g. a 429) freezes the animation for the rest of
// the reply, and 1.5s exceeds group-chat edit limits (20/min); fine for private chats.
var thinkingFrameEvery = 1500 * time.Millisecond

var thinkingFrames = []string{"Pensando.", "Pensando..", "Pensando..."}

// Thinking is an animated "Pensando..." placeholder shown while a reply is
// being produced. It is owned by one goroutine: call Finish or Stop from the
// goroutine that started it.
type Thinking struct {
	bot       Bot
	chatID    int64
	messageID int
	cancel    context.CancelFunc
	done      chan struct{}
}

// StartThinking sends the placeholder, then animates it until Finish, Stop, or
// ctx cancellation. Telegram failures are logged, never returned.
func StartThinking(ctx context.Context, bot Bot, chatID int64) *Thinking {
	t := &Thinking{bot: bot, chatID: chatID, done: make(chan struct{})}
	if msg, err := bot.SendMessage(ctx, tu.Message(tu.ID(chatID), thinkingFrames[0])); err != nil {
		slog.WarnContext(ctx, "telegram.thinking_send_failed", telemetry.Err(err))
	} else {
		t.messageID = msg.MessageID
	}

	animCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	go t.animate(animCtx)
	return t
}

func (t *Thinking) animate(ctx context.Context) {
	defer close(t.done)
	if t.messageID == 0 {
		return
	}
	frames := time.NewTicker(thinkingFrameEvery)
	defer frames.Stop()

	for frame := 1; ; {
		select {
		case <-ctx.Done():
			return
		case <-frames.C:
			text := thinkingFrames[frame%len(thinkingFrames)]
			frame++
			if _, err := t.bot.EditMessageText(ctx, tu.EditMessageText(tu.ID(t.chatID), t.messageID, text)); err != nil {
				if ctx.Err() == nil {
					slog.WarnContext(ctx, "telegram.thinking_animate_failed", telemetry.Err(err))
				}
				return
			}
		}
	}
}

func (t *Thinking) halt() {
	t.cancel()
	<-t.done
}

// Finish stops the animation and turns the placeholder into r, falling back to
// SendLong when there is no placeholder or it can no longer be edited.
func (t *Thinking) Finish(ctx context.Context, r Reply) error {
	t.halt()
	if t.messageID == 0 {
		return sendLong(ctx, t.bot, t.chatID, r)
	}
	messageID := t.messageID
	t.messageID = 0

	sends := longMessageParams(t.chatID, r)
	edit := htmlEdit(t.chatID, messageID, sends[0].Text)
	if len(sends) == 1 {
		edit = edit.WithReplyMarkup(Keyboard(r.Rows))
	}
	if _, err := t.bot.EditMessageText(ctx, edit); err != nil {
		slog.WarnContext(ctx, "telegram.thinking_edit_failed", telemetry.Err(err))
		t.deletePlaceholder(ctx, messageID)
		return sendLong(ctx, t.bot, t.chatID, r)
	}
	for _, p := range sends[1:] {
		if _, err := t.bot.SendMessage(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// Stop ends the animation and deletes the placeholder, for when the reply goes
// out another way or not at all. It is a no-op after Finish or a prior Stop.
func (t *Thinking) Stop(ctx context.Context) {
	t.halt()
	if t.messageID == 0 {
		return
	}
	t.deletePlaceholder(ctx, t.messageID)
	t.messageID = 0
}

func (t *Thinking) deletePlaceholder(ctx context.Context, messageID int) {
	if err := t.bot.DeleteMessage(ctx, tu.Delete(tu.ID(t.chatID), messageID)); err != nil {
		slog.WarnContext(ctx, "telegram.thinking_delete_failed", telemetry.Err(err))
	}
}
