package typesafe

import (
	"encoding/json"
	"testing"

	"github.com/shoenig/test/must"
)

func marshalCompact(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	must.NoError(t, err)
	return string(b)
}

func TestQuestionWireShape(t *testing.T) {
	// Request examples from https://docs.typesafe.ai/api.md.
	must.Eq(t, `{"type":"noul","instructions":"Does this convey urgency?"}`,
		marshalCompact(t, Noul("Does this convey urgency?")))

	must.Eq(t, `{"type":"noul","instructions":"Does this convey urgency?","criteria":{"false":"No urgency expressed","true":"Explicitly time-sensitive"}}`,
		marshalCompact(t, NoulWithCriteria("Does this convey urgency?", "Explicitly time-sensitive", "No urgency expressed")))

	must.Eq(t, `{"type":"noul","instructions":"q","criteria":{"true":"yes"}}`,
		marshalCompact(t, NoulWithCriteria("q", "yes", nil)), must.Sprint("nil side omitted"))

	must.Eq(t, `{"type":"noul","instructions":"q"}`,
		marshalCompact(t, NoulWithCriteria("q", nil, nil)), must.Sprint("both nil omits criteria"))

	must.Eq(t, `{"type":"choice","instructions":"Which team should handle this?","criteria":{"billing":"Payments, invoicing, refunds","sales":"Pricing, upgrades, new accounts","technical":"Bugs, outages, integrations"}}`,
		marshalCompact(t, Choice("Which team should handle this?", map[string]any{
			"billing":   "Payments, invoicing, refunds",
			"technical": "Bugs, outages, integrations",
			"sales":     "Pricing, upgrades, new accounts",
		})))

	must.Eq(t, `{"type":"choice","instructions":"tone","criteria":{"angry":null,"calm":null}}`,
		marshalCompact(t, Choice("tone", map[string]any{"calm": nil, "angry": nil})), must.Sprint("null descriptions kept"))

	must.Eq(t, `{"type":"score","instructions":"How frustrated is the customer?","criteria":["Calm","Frustrated","Very angry"]}`,
		marshalCompact(t, Score("How frustrated is the customer?", []any{"Calm", "Frustrated", "Very angry"})))

	must.Eq(t, `{"type":"noul"}`, marshalCompact(t, Noul(nil)), must.Sprint("nil instructions omitted"))

	must.Eq(t, `{"type":"noul","instructions":{"potential_duplicate":{"name":"John Smith"},"question":"Same person as `+"`potential_duplicate`"+`?"}}`,
		marshalCompact(t, Noul(map[string]any{
			"question":            "Same person as `potential_duplicate`?",
			"potential_duplicate": map[string]any{"name": "John Smith"},
		})), must.Sprint("structured instructions pass through"))
}

func TestValidateQuestions(t *testing.T) {
	must.EqError(t, validateQuestions(nil), "at least one question is required")
	must.EqError(t, validateQuestions(map[string]Question{}), "at least one question is required")

	must.NoError(t, validateQuestions(map[string]Question{
		"a": Noul("q"),
		"b": Choice("q", map[string]any{"x": nil}),
		"c": Score("q", []any{"low", "high"}),
	}))

	must.EqError(t, validateQuestions(map[string]Question{"f": Score("q", []any{"only"})}),
		`score question "f" has 1 criteria; at least two scores are required`)
	must.EqError(t, validateQuestions(map[string]Question{"f": Score("q", nil)}),
		`score question "f" has 0 criteria; at least two scores are required`)

	must.EqError(t, validateQuestions(map[string]Question{"z": {}}),
		`question "z" is the zero Question; use Noul, Choice, or Score`)
}
