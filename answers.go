package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Usage is the token usage for one request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Model describes a model available to the account.
type Model struct {
	// Name is accepted by Request.Model and Config.Model.
	Name        string `json:"name"`
	Description string `json:"description"`
	// ReleaseDate is a date string; the API does not pin its format.
	ReleaseDate string `json:"release_date"`
}

// Answer is one answer from Client.SystemOne. It is implemented by
// NoulAnswer, ChoiceAnswer, ScoreAnswer, and UnknownAnswer; switch on the
// concrete type, or use Result.DecodeAnswers for a typed struct.
type Answer interface{ answer() }

// NoulAnswer answers a Noul question.
type NoulAnswer struct {
	// Noul is the probability of yes, from 0 to 1.
	Noul float64 `json:"noul"`
}

// ChoiceAnswer answers a Choice question.
type ChoiceAnswer struct {
	// Choice is the highest-probability label.
	Choice string `json:"choice"`
	// Confidence is the model's certainty in Choice, from 0 to 1.
	Confidence float64 `json:"confidence"`
	// Probabilities maps every label to its probability; they sum to 1.
	Probabilities map[string]float64 `json:"probabilities"`
}

// ScoreAnswer answers a Score question.
type ScoreAnswer struct {
	// Score is the probability-weighted level; it may fall between levels.
	Score float64 `json:"score"`
	// Confidence is the model's certainty in Score, from 0 to 1.
	Confidence float64 `json:"confidence"`
	// Legend maps each level index to the description supplied in the question.
	Legend map[int]any `json:"legend"`
	// Probabilities maps each level index to its probability; they sum to 1.
	Probabilities map[int]float64 `json:"probabilities"`
}

// UnknownAnswer is an answer whose type this version of the package does not
// model. Raw is the complete answer object.
type UnknownAnswer struct {
	Type string
	Raw  json.RawMessage
}

func (NoulAnswer) answer()    {}
func (ChoiceAnswer) answer()  {}
func (ScoreAnswer) answer()   {}
func (UnknownAnswer) answer() {}

// checkAnswerType rejects an answer object whose type tag is not want, so a
// caller's typed struct fails loudly when a question name maps to the wrong
// answer kind.
func checkAnswerType(b []byte, want string) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	if tag.Type != want {
		return fmt.Errorf("typesafe: expected %s answer, got %q", want, tag.Type)
	}
	return nil
}

// UnmarshalJSON decodes a noul answer object and rejects any other type.
func (a *NoulAnswer) UnmarshalJSON(b []byte) error {
	if err := checkAnswerType(b, kindNoul); err != nil {
		return err
	}
	type plain NoulAnswer
	return json.Unmarshal(b, (*plain)(a))
}

// UnmarshalJSON decodes a choice answer object and rejects any other type.
func (a *ChoiceAnswer) UnmarshalJSON(b []byte) error {
	if err := checkAnswerType(b, kindChoice); err != nil {
		return err
	}
	type plain ChoiceAnswer
	return json.Unmarshal(b, (*plain)(a))
}

// UnmarshalJSON decodes a score answer object and rejects any other type.
// encoding/json decodes the "0", "1", ... object keys into int map keys.
func (a *ScoreAnswer) UnmarshalJSON(b []byte) error {
	if err := checkAnswerType(b, kindScore); err != nil {
		return err
	}
	type plain ScoreAnswer
	return json.Unmarshal(b, (*plain)(a))
}

// Result is the response to Client.SystemOne.
type Result struct {
	// Model is the model that answered.
	Model string
	// Answers holds one Answer per question, keyed by the question name.
	Answers map[string]Answer
	// Usage is the token usage for the request.
	Usage Usage
	// RequestID is the x-typesafe-request-id header, or "".
	RequestID string

	// answers is the raw answers object, for DecodeAnswers.
	answers json.RawMessage
}

// UnmarshalJSON decodes the response envelope and each tagged answer.
func (r *Result) UnmarshalJSON(b []byte) error {
	var wire struct {
		Model   string          `json:"model"`
		Answers json.RawMessage `json:"answers"`
		Usage   Usage           `json:"usage"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if len(wire.Answers) > 0 {
		if err := json.Unmarshal(wire.Answers, &raw); err != nil {
			return fmt.Errorf("answers: %w", err)
		}
	}
	answers := make(map[string]Answer, len(raw))
	for name, rb := range raw {
		a, err := decodeAnswer(rb)
		if err != nil {
			return fmt.Errorf("answer %q: %w", name, err)
		}
		answers[name] = a
	}
	r.Model, r.Answers, r.Usage, r.answers = wire.Model, answers, wire.Usage, wire.Answers
	return nil
}

func decodeAnswer(b json.RawMessage) (Answer, error) {
	var tag struct {
		Type *string `json:"type"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return nil, err
	}
	if tag.Type == nil {
		return nil, errors.New("missing type")
	}
	switch *tag.Type {
	case kindNoul:
		var a NoulAnswer
		return a, json.Unmarshal(b, &a)
	case kindChoice:
		var a ChoiceAnswer
		return a, json.Unmarshal(b, &a)
	case kindScore:
		var a ScoreAnswer
		return a, json.Unmarshal(b, &a)
	}
	return UnknownAnswer{Type: *tag.Type, Raw: append(json.RawMessage(nil), b...)}, nil
}

// DecodeAnswers unmarshals the answers object into dst, a pointer to a struct
// whose fields are NoulAnswer, ChoiceAnswer, or ScoreAnswer tagged with the
// question name (`json:"dept"`). A field whose answer has a different type
// returns an error; a question name absent from the response leaves its
// field zero, as json.Unmarshal does.
func (r *Result) DecodeAnswers(dst any) error {
	if len(r.answers) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.answers, dst); err != nil {
		return fmt.Errorf("typesafe: decode answers: %w", err)
	}
	return nil
}

// unknownAnswerNames returns the sorted names of UnknownAnswer entries, or nil.
func (r *Result) unknownAnswerNames() []string {
	var names []string
	for name, a := range r.Answers {
		if _, ok := a.(UnknownAnswer); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
