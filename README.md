# typesafe-go

Go client for the TypeSafe AI System One API. Internal to North Pole Security.

```go
client, err := typesafe.New(typesafe.Config{}) // reads TYPESAFE_API_KEY
res, err := client.SystemOne(ctx, typesafe.Request{
    State: "I was charged twice. Please help.",
    Questions: map[string]typesafe.Question{
        "billing": typesafe.Noul("Is this about billing?"),
        "tone":    typesafe.Choice("What is the tone?", map[string]any{"calm": nil, "angry": nil}),
    },
})
if a, ok := res.Answers["tone"].(typesafe.ChoiceAnswer); ok {
    fmt.Println(a.Choice, a.Confidence)
}
```

See `PORTING.md` for how this maps onto the official JS SDK and where it diverges.
