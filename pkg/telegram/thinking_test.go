package telegram

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mymmrac/telego"
)

type botCall struct {
	method    string
	text      string
	parseMode string
	keyboard  bool
	messageID int
}

type fakeBot struct {
	mu        sync.Mutex
	calls     []botCall
	sendErr   error
	editErr   error
	nextMsgID int
}

func (b *fakeBot) record(c botCall) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, c)
}

func (b *fakeBot) SendMessage(_ context.Context, p *telego.SendMessageParams) (*telego.Message, error) {
	b.record(botCall{method: "send", text: p.Text, parseMode: p.ParseMode, keyboard: p.ReplyMarkup != nil})
	if b.sendErr != nil {
		return nil, b.sendErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextMsgID++
	return &telego.Message{MessageID: b.nextMsgID}, nil
}

func (b *fakeBot) EditMessageText(_ context.Context, p *telego.EditMessageTextParams) (*telego.Message, error) {
	b.record(botCall{method: "edit", text: p.Text, parseMode: p.ParseMode, keyboard: p.ReplyMarkup != nil, messageID: p.MessageID})
	if b.editErr != nil {
		return nil, b.editErr
	}
	return &telego.Message{MessageID: p.MessageID}, nil
}

func (b *fakeBot) DeleteMessage(_ context.Context, p *telego.DeleteMessageParams) error {
	b.record(botCall{method: "delete", messageID: p.MessageID})
	return nil
}

func (b *fakeBot) snapshot() []botCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]botCall(nil), b.calls...)
}

func (b *fakeBot) count(method string) int {
	n := 0
	for _, c := range b.snapshot() {
		if c.method == method {
			n++
		}
	}
	return n
}

func (b *fakeBot) setErrs(send, edit error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sendErr, b.editErr = send, edit
}

func (b *fakeBot) edits() []string {
	var out []string
	for _, c := range b.snapshot() {
		if c.method == "edit" {
			out = append(out, c.text)
		}
	}
	return out
}

func TestStartThinkingSendsPlaceholder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		defer th.Stop(context.Background())

		calls := bot.snapshot()
		if len(calls) != 1 || calls[0].method != "send" || calls[0].text != "Pensando." || calls[0].parseMode != "" {
			t.Fatalf("calls = %+v, want one plain send of Pensando.", calls)
		}
	})
}

func TestThinkingCyclesFrames(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		time.Sleep(4*thinkingFrameEvery + time.Millisecond)
		synctest.Wait()
		th.Stop(context.Background())

		want := []string{"Pensando..", "Pensando...", "Pensando.", "Pensando.."}
		if got := bot.edits(); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("edits = %v, want %v", got, want)
		}
	})
}

func TestFailedFrameEditStopsLaterEdits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		bot.setErrs(nil, errors.New("too many requests"))
		time.Sleep(10 * thinkingFrameEvery)
		synctest.Wait()
		th.Stop(context.Background())

		if n := bot.count("edit"); n != 1 {
			t.Fatalf("edits = %d, want 1", n)
		}
	})
}

func TestFinishEditsPlaceholderAndStopsAnimation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		time.Sleep(2 * thinkingFrameEvery)

		reply := Reply{Text: "<b>done</b>", Rows: [][]Button{{{Text: "Ok", Data: "ok"}}}}
		if err := th.Finish(context.Background(), reply); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		after := bot.snapshot()
		last := after[len(after)-1]
		if last.method != "edit" || last.text != reply.Text || last.parseMode != telego.ModeHTML || !last.keyboard || last.messageID != 1 {
			t.Fatalf("last call = %+v, want HTML edit of placeholder with keyboard", last)
		}

		time.Sleep(10 * thinkingFrameEvery)
		synctest.Wait()
		if n := len(bot.snapshot()); n != len(after) {
			t.Fatalf("calls after Finish: %d -> %d", len(after), n)
		}
	})
}

func TestFinishLongReplySendsRemainingChunks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		reply := Reply{Text: strings.Repeat("a ", MaxLen), Rows: [][]Button{{{Text: "Ok", Data: "ok"}}}}
		if err := th.Finish(context.Background(), reply); err != nil {
			t.Fatalf("Finish: %v", err)
		}

		chunks := Split(reply.Text)
		calls := bot.snapshot()[1:]
		if len(chunks) < 2 || len(calls) != len(chunks) {
			t.Fatalf("calls = %d, chunks = %d", len(calls), len(chunks))
		}
		if calls[0].method != "edit" || calls[0].text != chunks[0] || calls[0].keyboard {
			t.Fatalf("first = %+v, want keyboard-less edit of first chunk", calls[0])
		}
		for i, c := range calls[1:] {
			wantKB := i == len(calls)-2
			if c.method != "send" || c.text != chunks[i+1] || c.parseMode != telego.ModeHTML || c.keyboard != wantKB {
				t.Fatalf("chunk %d = %+v", i+1, c)
			}
		}
	})
}

func TestFinishFallsBackWhenEditFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		bot.setErrs(nil, errors.New("message to edit not found"))
		if err := th.Finish(context.Background(), Reply{Text: "hi"}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		calls := bot.snapshot()
		n := len(calls)
		if calls[n-2].method != "delete" || calls[n-2].messageID != 1 {
			t.Fatalf("calls = %+v, want placeholder deleted", calls)
		}
		if calls[n-1].method != "send" || calls[n-1].text != "hi" {
			t.Fatalf("calls = %+v, want reply sent", calls)
		}
	})
}

func TestFinishSendsNormallyWhenPlaceholderFailed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{sendErr: errors.New("boom")}
		th := StartThinking(context.Background(), bot, 7)
		bot.setErrs(nil, nil)

		if err := th.Finish(context.Background(), Reply{Text: "hi"}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		calls := bot.snapshot()
		last := calls[len(calls)-1]
		if last.method != "send" || last.text != "hi" || bot.count("edit") != 0 {
			t.Fatalf("calls = %+v, want plain send", calls)
		}
	})
}

func TestFinishReturnsDeliveryError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		bot.setErrs(errors.New("down"), errors.New("gone"))
		if err := th.Finish(context.Background(), Reply{Text: "hi"}); err == nil {
			t.Fatal("want delivery error")
		}
	})
}

func TestStopDeletesPlaceholderOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		th.Stop(context.Background())
		th.Stop(context.Background())
		if n := bot.count("delete"); n != 1 {
			t.Fatalf("deletes = %d, want 1", n)
		}
	})
}

func TestStopAfterFinishIsNoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		if err := th.Finish(context.Background(), Reply{Text: "hi"}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		before := len(bot.snapshot())
		th.Stop(context.Background())
		if n := len(bot.snapshot()); n != before {
			t.Fatalf("Stop after Finish made calls: %+v", bot.snapshot()[before:])
		}
	})
}

func TestFinishAfterStopSendsNewMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		th := StartThinking(context.Background(), bot, 7)
		th.Stop(context.Background())
		if err := th.Finish(context.Background(), Reply{Text: "hi"}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		calls := bot.snapshot()
		last := calls[len(calls)-1]
		if last.method != "send" || last.text != "hi" || last.parseMode != telego.ModeHTML || bot.count("edit") != 0 {
			t.Fatalf("calls = %+v, want reply sent as a new message", calls)
		}
	})
}

func TestContextCancelStopsAnimation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bot := &fakeBot{}
		ctx, cancel := context.WithCancel(context.Background())
		th := StartThinking(ctx, bot, 7)
		cancel()
		<-th.done

		before := len(bot.snapshot())
		time.Sleep(10 * thinkingFrameEvery)
		synctest.Wait()
		if n := len(bot.snapshot()); n != before {
			t.Fatalf("animation kept running after cancel: %d -> %d", before, n)
		}
	})
}
