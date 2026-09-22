package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	kindNoul   = "noul"
	kindChoice = "choice"
	kindScore  = "score"
)

// Question is one typed question for Client.SystemOne. Build one with Noul,
// NoulWithCriteria, Choice, or Score; the zero value is invalid.
//
// Instructions and every description accept what the API accepts: a string,
// a map or slice that marshals to a JSON object or array, or nil.
type Question struct {
	kind         string
	instructions any
	criteria     any // nil; map[string]any for noul and choice; []any for score
}

// Noul is a yes/no question. The answer is a NoulAnswer with the probability
// of yes.
func Noul(instructions any) Question {
	return Question{kind: kindNoul, instructions: instructions}
}

// NoulWithCriteria is a yes/no question with descriptions of what yes and no
// mean; either may be nil.
func NoulWithCriteria(instructions any, yes, no any) Question {
	q := Noul(instructions)
	c := map[string]any{}
	if yes != nil {
		c["true"] = yes
	}
	if no != nil {
		c["false"] = no
	}
	if len(c) > 0 {
		q.criteria = c
	}
	return q
}

// Choice selects one option from options, a map of label to description; a
// nil description leaves the label undescribed. The answer is a ChoiceAnswer.
func Choice(instructions any, options map[string]any) Question {
	return Question{kind: kindChoice, instructions: instructions, criteria: options}
}

// Score rates the state against levels, an ordered rubric indexed from zero;
// a nil level is undescribed. At least two levels are required. The answer is
// a ScoreAnswer.
func Score(instructions any, levels []any) Question {
	return Question{kind: kindScore, instructions: instructions, criteria: levels}
}

type questionWire struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

// MarshalJSON emits the API's tagged object. A nil interface is omitted, so
// Noul(nil) sends no instructions (the JS SDK sends instructions: null; the
// API accepts both). Choice and Score store the caller's map or slice as-is,
// so a nil map or slice marshals as criteria: null.
func (q Question) MarshalJSON() ([]byte, error) {
	return json.Marshal(questionWire{Type: q.kind, Instructions: q.instructions, Criteria: q.criteria})
}

// validateQuestions applies the checks the JS SDK makes before sending: at
// least one question, and at least two score levels. Server-side limits
// (255 choice options, 10 score levels) are left to the server's 422.
func validateQuestions(qs map[string]Question) error {
	if len(qs) == 0 {
		return errors.New("at least one question is required")
	}
	for name, q := range qs {
		switch q.kind {
		case kindNoul, kindChoice:
		case kindScore:
			levels, _ := q.criteria.([]any)
			if len(levels) < 2 {
				return fmt.Errorf("score question %q has %d criteria; at least two scores are required", name, len(levels))
			}
		default:
			return fmt.Errorf("question %q is the zero Question; use Noul, Choice, or Score", name)
		}
	}
	return nil
}
