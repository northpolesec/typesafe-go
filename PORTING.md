# PORTING.md

Ledger of the `typesafe-sdk-js` port into this module, in the format of `chat-go/PORTING.md`. Upstream: https://github.com/typesafe-ai/typesafe-sdk-js `src/`. Wire contract: https://docs.typesafe.ai/api.md. Every change to this module that alters a row below updates it in the same PR.

## Upstream baseline

Ported from typesafe-sdk-js `66880cc` (v0.6.0), cross-checked against typesafe-sdk-python v0.7.1. Last checked 2026-10-06: JS unchanged; Python v0.7.2 (`f078f1e`) adds only an optional `http2` packaging extra, which needs nothing here since `net/http` negotiates HTTP/2 over TLS. Wire contract checked against `openapi.json` version 0.2.0 (`/v1/systemone`, `/v1/models`); every schema, required field, and the `noul`/`choice`/`score` discriminator match this module. When re-checking, diff fresh clones against these commits and diff `openapi.json` against version 0.2.0.

## Global divergences

- `fetch` → `HTTPDoer` (`*http.Client` satisfies it); nil → package-owned `&http.Client{}`, never `http.DefaultClient`
- `Logger` interface + `TYPESAFE_LOG_LEVEL` → `*slog.Logger`; nil → `slog.DiscardHandler`; the caller's logger carries its level
- `AbortSignal` → `ctx`; a ctx deadline is a total budget across attempts, not a per-attempt timeout
- Seven error classes → one `*Error` + three sentinels (`ErrAuthentication`, `ErrRateLimited`, `ErrOverloaded`) + stdlib `net.Error` / `context` errors
- 9-knob `RetryPolicy` → 3 fields; jitter (25%) and Retry-After cap (60s) are constants; `nil` = defaults, `&RetryPolicy{}` = off, fields literal
- Per-call `RequestOptions` (timeout, retry, headers, signal) dropped; use `ctx` or a second `Client`
- `defaultHeaders` dropped; wrap `HTTPClient` for extra headers
- `dangerouslyAllowBrowser` dropped (no browser runtime)
- `APIPromise.asResponse()/withResponse()` → `Result.RequestID` and `Error.RequestID`; `Models` does not expose a request id
- `instructions: null` (JS `noul()` default) → key omitted when nil; API accepts both
- Request and response bodies are never logged (JS logs them at `debug`); `Error()` never includes the raw body
- 255-option and 10-level limits left to the server's 422 (neither upstream SDK checks them); score minimum follows JS and api.md (≥2), not Python or openapi.json (`minItems: 1`)
- `Request.State` nil marshals as `state: null`, as JS allows; `openapi.json` excludes null (expect a 422; not verified live)
- Score `instructions` is optional here, per `openapi.json` and both SDKs; api.md marks it required
- `Question` is opaque with constructors; upstream is a plain tagged object
- `Answer` is a sealed interface (`NoulAnswer | ChoiceAnswer | ScoreAnswer | UnknownAnswer`); upstream infers types from the question literal; unknown answer types are kept as `UnknownAnswer` and named in one WARN (Python drops them with a warning; JS passes them through)
- `const Version` bumped on tag, not `debug.ReadBuildInfo()`
- Response body capped at 8 MiB with an explicit error; upstream has no cap
- `Config.APIKey` is `TrimSpace`d before validation (Python strips too; JS does not); interior whitespace and non-printable bytes are still rejected
- `DefaultBaseURL` and `DefaultModel` are exported constants

## Files

| Upstream file | Go file | Tests | Divergences |
| --- | --- | --- | --- |
| `src/client.ts` | `client.go` | `client_test.go`, `transport_test.go` | `TypeSafeClientConfig` → `Config`; `#request`/`fetchWithRetries`/`attempt`/`backOff` → `do`/`attempt`/`sleep`; `bufferResponse` → `io.ReadAll(io.LimitReader)`; `#requestCount` log tag dropped (no mutable state); `mergeHeaders` dropped with `defaultHeaders`; API key validated in `New` (Python's rule); `BaseURL` must parse as absolute http(s) |
| `src/types.ts` | `questions.go`, `answers.go`, `client.go` | `questions_test.go`, `answers_test.go` | `EntryType`/`JsonValue` → `any`; `Questions` → `map[string]Question`; `SystemOneRequest` → `Request`; `SystemOneResult` → `Result`; `ResultFor<T>` inference → `DecodeAnswers` into a caller struct; `Usage` ints; `ModelCard` → `Model` |
| `src/questions.ts` | `questions.go` | `questions_test.go` | `noul(instructions=null, criteria?)` → `Noul` + `NoulWithCriteria(yes, no)`; `score(instructions, [..])` → `Score(instructions, []any)`; `choice` map shape enforced by the constructor signature; `validateQuestions` keeps the two JS checks |
| `src/errors.ts` | `errors.go` | `errors_test.go` | `APIError` + 7 subclasses → `*Error` + `Is`; `RateLimitError.retryAfterMs` → `Error.RetryAfter` on every `*Error`; `APIConnectionError`/`APITimeoutError`/`APIUserAbortError` → wrapped `net`/`context` errors; `describe()` never emits the raw body |
| `src/retry.ts` | `retry.go` | `retry_test.go` | `retryDelayMs(attempt, headers, policy, random)` → `backoffDelay(n, h, p, r, now)`; `parseRetryAfter` returns `(d, ok)` so `retry-after-ms: 0` is honored; HTTP-date via `http.ParseTime`; `sleep(ms, signal)` → `sleep(ctx, d)` with a stopped timer |
| `src/resources/models.ts` | `client.go` (`Models`) | `transport_test.go` | `Models` resource class → method on `Client`; `unwrapModels` shape check kept |
| `src/api-promise.ts` | `client.go` (`do` return), `answers.go` (`Result.RequestID`) | `transport_test.go` | `APIPromise` dropped; request id returned from `do` |
| `src/env.ts` | `client.go` (`fromCodeOrEnv`) | `client_test.go` | same three env vars; `TYPESAFE_LOG_LEVEL` dropped |
| `src/logging.ts` | `client.go` (`slog` calls) | `transport_test.go` | `redactHeaders` unnecessary: headers are never logged |
| `src/runtime.ts` | `client.go` (`runtimeHeader`) | `transport_test.go` | `describeRuntime()` → `go/<runtime.Version()> <GOOS>/<GOARCH>`; `isBrowser()` dropped |
| `src/version.ts` | `doc.go` | — | `const Version` |
