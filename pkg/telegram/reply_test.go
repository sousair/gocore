package telegram

import "testing"

func TestEscape(t *testing.T) {
	cases := map[string]string{
		"AT&T":        "AT&amp;T",
		"EC < 1.2":    "EC &lt; 1.2",
		"temp > 26":   "temp &gt; 26",
		"a<b>&c":      "a&lt;b&gt;&amp;c",
		"plain text":  "plain text",
		"":            "",
	}
	for in, want := range cases {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}
