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

## Git hooks

Install [lefthook](https://github.com/evilmartians/lefthook) to run CI's checks before each push:

```shell
brew install lefthook
lefthook install
```

The pre-push hook runs `make ci` and `make lint/workflows` (actionlint and zizmor, which needs Docker and `gh auth login`). Skip it once with `git push --no-verify`.
