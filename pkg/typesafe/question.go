package typesafe

import (
	"cmp"
	"image"
	"math"
	"slices"
)

// QuestionType selects how System One answers a question.
type QuestionType string

const (
	QuestionChoice QuestionType = "choice"
	QuestionNoul   QuestionType = "noul"
	QuestionScore  QuestionType = "score"
	// QuestionBox needs ModelJev114 and an image in the request.
	QuestionBox QuestionType = "box"
)

// Question is one entry of a System One request, keyed by id in the map
// passed to Client.SystemOne.
type Question struct {
	Type         QuestionType `json:"type"`
	Instructions any          `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

// Choice asks for one of the options named by criteria's keys.
func Choice(instructions any, criteria map[string]any) Question {
	return Question{Type: QuestionChoice, Instructions: instructions, Criteria: criteria}
}

// Noul asks for the probability that instructions hold (yes/no).
func Noul(instructions any) Question {
	return Question{Type: QuestionNoul, Instructions: instructions}
}

// Score asks for a rating on the given ordered levels.
func Score(instructions any, levels []any) Question {
	return Question{Type: QuestionScore, Instructions: instructions, Criteria: levels}
}

// Box asks where in an image the thing described by instructions is. It
// always returns one box, never "not found": pair it with a Noul.
func Box(instructions any) Question {
	return Question{Type: QuestionBox, Instructions: instructions}
}

// Edges is a Box answer as fractions 0-1 from the image's top-left corner:
// Left and Right of its width, Top and Bottom of its height.
type Edges struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
}

// Pixels converts the edges to a pixel rectangle on a width x height image,
// clamped to its bounds. Pass the size of the image that was sent.
func (e Edges) Pixels(width, height int) image.Rectangle {
	px := func(f float64, size int) int {
		return min(max(int(math.Round(f*float64(size))), 0), size)
	}
	return image.Rect(px(e.Left, width), px(e.Top, height), px(e.Right, width), px(e.Bottom, height))
}

// Answer is the model's reply to one question; only the fields for its Type
// are populated.
type Answer struct {
	Type          QuestionType       `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	Noul          float64            `json:"noul"`
	Score         float64            `json:"score"`
	Box           Edges              `json:"box"`
}

type Ranked struct {
	Option      string
	Probability float64
}

// Ranked returns the options by descending probability, ties broken by name.
func (a Answer) Ranked() []Ranked {
	out := make([]Ranked, 0, len(a.Probabilities))
	for option, p := range a.Probabilities {
		out = append(out, Ranked{Option: option, Probability: p})
	}
	slices.SortFunc(out, func(a, b Ranked) int {
		return cmp.Or(cmp.Compare(b.Probability, a.Probability), cmp.Compare(a.Option, b.Option))
	})
	return out
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// Cost is the USD price OpenRouter reports; TypeSafe leaves it zero.
	Cost float64 `json:"cost"`
}

func (u Usage) costUSD() float64 {
	if u.Cost > 0 {
		return u.Cost
	}
	return float64(u.InputTokens) * inputUSDPerToken
}

type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}
