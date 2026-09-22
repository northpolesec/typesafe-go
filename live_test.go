package typesafe_test

import (
	"os"
	"testing"

	"github.com/northpolesec/typesafe-go"
	"github.com/shoenig/test/must"
)

// liveClient returns a client for the real API, or skips the test when
// TYPESAFE_API_KEY is unset. One real round trip is the port's live gate.
func liveClient(t *testing.T) *typesafe.Client {
	t.Helper()
	if os.Getenv("TYPESAFE_API_KEY") == "" {
		t.Skip("TYPESAFE_API_KEY not set")
	}
	c, err := typesafe.New(typesafe.Config{})
	must.NoError(t, err)
	return c
}

func TestLive_SystemOne(t *testing.T) {
	c := liveClient(t)
	res, err := c.SystemOne(t.Context(), typesafe.Request{
		State: "Help! My payouts have been failing for 3 days.",
		Questions: map[string]typesafe.Question{
			"is_urgent":   typesafe.Noul("Does this convey urgency?"),
			"department":  typesafe.Choice("Which team should handle this?", map[string]any{"billing": "Payments, invoicing, refunds", "technical": "Bugs, outages, integrations", "sales": "Pricing, upgrades, new accounts"}),
			"frustration": typesafe.Score("How frustrated is the customer?", []any{"Calm", "Frustrated", "Very angry"}),
		},
	})
	must.NoError(t, err)
	must.NotEq(t, "", res.RequestID)
	must.NotEq(t, "", res.Model)
	must.Positive(t, res.Usage.InputTokens)

	urgent, ok := res.Answers["is_urgent"].(typesafe.NoulAnswer)
	must.True(t, ok)
	must.Between(t, 0, urgent.Noul, 1)

	dept, ok := res.Answers["department"].(typesafe.ChoiceAnswer)
	must.True(t, ok)
	must.MapContainsKey(t, dept.Probabilities, dept.Choice)
	must.Between(t, 0, dept.Confidence, 1)

	fr, ok := res.Answers["frustration"].(typesafe.ScoreAnswer)
	must.True(t, ok)
	must.Eq(t, map[int]any{0: "Calm", 1: "Frustrated", 2: "Very angry"}, fr.Legend)
	must.Between(t, 0, fr.Score, 2)
}

func TestLive_Models(t *testing.T) {
	c := liveClient(t)
	models, err := c.Models(t.Context())
	must.NoError(t, err)
	must.Positive(t, len(models))
	must.NotEq(t, "", models[0].Name)
}
