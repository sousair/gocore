package typesafe

import (
	"cmp"
	"slices"
)

// QuestionType selects how System One answers a question.
type QuestionType string

const (
	QuestionChoice QuestionType = "choice"
	QuestionNoul   QuestionType = "noul"
	QuestionScore  QuestionType = "score"
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

// Answer is the model's reply to one question; only the fields for its Type
// are populated.
type Answer struct {
	Type          QuestionType       `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	Noul          float64            `json:"noul"`
	Score         float64            `json:"score"`
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
