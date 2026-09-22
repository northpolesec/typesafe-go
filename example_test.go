package typesafe_test

import (
	"context"
	"fmt"
	"log"

	"github.com/northpolesec/typesafe-go"
)

func Example() {
	client, err := typesafe.New(typesafe.Config{APIKey: "sk-example"})
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.SystemOne(context.Background(), typesafe.Request{
		State: "Help! My payouts have been failing for 3 days.",
		Questions: map[string]typesafe.Question{
			"is_urgent": typesafe.Noul("Does this convey urgency?"),
			"department": typesafe.Choice("Which team should handle this?", map[string]any{
				"billing":   "Payments, invoicing, refunds",
				"technical": "Bugs, outages, integrations",
				"sales":     "Pricing, upgrades, new accounts",
			}),
			"frustration": typesafe.Score("How frustrated is the customer?", []any{"Calm", "Frustrated", "Very angry"}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Dynamic access: switch on the concrete answer type.
	if a, ok := res.Answers["department"].(typesafe.ChoiceAnswer); ok {
		fmt.Println(a.Choice, a.Confidence)
	}

	// Typed access: decode into a struct keyed by question name.
	var out struct {
		Urgent      typesafe.NoulAnswer  `json:"is_urgent"`
		Frustration typesafe.ScoreAnswer `json:"frustration"`
	}
	if err := res.DecodeAnswers(&out); err != nil {
		log.Fatal(err)
	}
	fmt.Println(out.Urgent.Noul > 0.5, out.Frustration.Score)
}
