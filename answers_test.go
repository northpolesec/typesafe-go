package typesafe

import (
	"encoding/json"
	"testing"

	"github.com/shoenig/test/must"
)

// Response examples from https://docs.typesafe.ai/api.md.
const (
	noulResponse = `{"model":"jev-1.13.0","answers":{"is_urgent":{"type":"noul","noul":0.95}},"usage":{"input_tokens":307,"output_tokens":20}}`

	choiceResponse = `{"model":"jev-1.13.0","answers":{"department":{"type":"choice","choice":"billing","probabilities":{"billing":0.88,"technical":0.12,"sales":0.0},"confidence":0.81}},"usage":{"input_tokens":318,"output_tokens":34}}`

	scoreResponse = `{"model":"jev-1.13.0","answers":{"frustration":{"type":"score","score":1.05,"legend":{"0":"Calm","1":"Frustrated","2":"Very angry"},"probabilities":{"0":0.0,"1":0.95,"2":0.05},"confidence":0.92}},"usage":{"input_tokens":304,"output_tokens":18}}`
)

func TestResultUnmarshalNoul(t *testing.T) {
	var r Result
	must.NoError(t, json.Unmarshal([]byte(noulResponse), &r))
	must.Eq(t, "jev-1.13.0", r.Model)
	must.Eq(t, Usage{InputTokens: 307, OutputTokens: 20}, r.Usage)
	// Answers holds the Answer interface, so the expected value is converted
	// to Answer for the generic must.Eq.
	must.Eq(t, Answer(NoulAnswer{Noul: 0.95}), r.Answers["is_urgent"])
}

func TestResultUnmarshalChoice(t *testing.T) {
	var r Result
	must.NoError(t, json.Unmarshal([]byte(choiceResponse), &r))
	must.Eq(t, Answer(ChoiceAnswer{
		Choice:        "billing",
		Confidence:    0.81,
		Probabilities: map[string]float64{"billing": 0.88, "technical": 0.12, "sales": 0},
	}), r.Answers["department"])
}

func TestResultUnmarshalScore(t *testing.T) {
	var r Result
	must.NoError(t, json.Unmarshal([]byte(scoreResponse), &r))
	must.Eq(t, Answer(ScoreAnswer{
		Score:         1.05,
		Confidence:    0.92,
		Legend:        map[int]any{0: "Calm", 1: "Frustrated", 2: "Very angry"},
		Probabilities: map[int]float64{0: 0, 1: 0.95, 2: 0.05},
	}), r.Answers["frustration"])
}

func TestResultUnmarshalUnknownType(t *testing.T) {
	body := `{"model":"m","answers":{"k":{"type":"vector","values":[1,2]},"n":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1,"output_tokens":1}}`
	var r Result
	must.NoError(t, json.Unmarshal([]byte(body), &r))
	must.Eq(t, Answer(UnknownAnswer{Type: "vector", Raw: json.RawMessage(`{"type":"vector","values":[1,2]}`)}), r.Answers["k"])
	must.Eq(t, Answer(NoulAnswer{Noul: 0.5}), r.Answers["n"])
	must.Eq(t, []string{"k"}, r.unknownAnswerNames())
}

func TestResultUnmarshalMissingType(t *testing.T) {
	var r Result
	err := json.Unmarshal([]byte(`{"model":"m","answers":{"k":{"noul":0.5}}}`), &r)
	must.EqError(t, err, `answer "k": missing type`)
}

func TestResultUnmarshalEmptyAnswers(t *testing.T) {
	var r Result
	must.NoError(t, json.Unmarshal([]byte(`{"model":"m","usage":{"input_tokens":0,"output_tokens":0}}`), &r))
	must.Eq(t, 0, len(r.Answers))
	must.Nil(t, r.unknownAnswerNames())
	must.NoError(t, r.DecodeAnswers(&struct{}{}))
}

func TestDecodeAnswers(t *testing.T) {
	body := `{"model":"m","answers":{"dept":{"type":"choice","choice":"billing","confidence":0.9,"probabilities":{"billing":0.9,"sales":0.1}},"urgent":{"type":"noul","noul":0.7},"mood":{"type":"score","score":1.0,"confidence":0.8,"legend":{"0":"a","1":"b"},"probabilities":{"0":0,"1":1}}},"usage":{"input_tokens":1,"output_tokens":1}}`
	var r Result
	must.NoError(t, json.Unmarshal([]byte(body), &r))

	var out struct {
		Dept   ChoiceAnswer `json:"dept"`
		Urgent NoulAnswer   `json:"urgent"`
		Mood   ScoreAnswer  `json:"mood"`
		Absent NoulAnswer   `json:"absent"`
	}
	must.NoError(t, r.DecodeAnswers(&out))
	must.Eq(t, "billing", out.Dept.Choice)
	must.Eq(t, 0.9, out.Dept.Confidence)
	must.Eq(t, 0.7, out.Urgent.Noul)
	must.Eq(t, map[int]float64{0: 0, 1: 1}, out.Mood.Probabilities)
	must.Eq(t, NoulAnswer{}, out.Absent, must.Sprint("missing names stay zero"))

	var wrong struct {
		Dept NoulAnswer `json:"dept"`
	}
	must.EqError(t, r.DecodeAnswers(&wrong), `typesafe: decode answers: typesafe: expected noul answer, got "choice"`)
}

func TestAnswerUnmarshalTypeCheck(t *testing.T) {
	var c ChoiceAnswer
	must.EqError(t, json.Unmarshal([]byte(`{"type":"score","score":1}`), &c), `typesafe: expected choice answer, got "score"`)
	var s ScoreAnswer
	must.EqError(t, json.Unmarshal([]byte(`{"score":1}`), &s), `typesafe: expected score answer, got ""`)
	var n NoulAnswer
	must.NoError(t, json.Unmarshal([]byte(`{"type":"noul","noul":0.1}`), &n))
	must.Eq(t, 0.1, n.Noul)
}
