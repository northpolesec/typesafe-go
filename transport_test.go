package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

// fastRetry keeps retry tests in the millisecond range.
var fastRetry = RetryPolicy{MaxRetries: 2, BackoffInitial: time.Millisecond, BackoffMax: time.Millisecond}

type recorded struct {
	requests atomic.Int32
	last     atomic.Pointer[http.Request]
	lastBody atomic.Pointer[[]byte]
}

// newServer returns a client pointed at an httptest.Server that runs handler
// after recording the request.
func newServer(t *testing.T, cfg Config, handler http.HandlerFunc) (*Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		rec.lastBody.Store(&body)
		rec.last.Store(r.Clone(r.Context()))
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	if cfg.APIKey == "" {
		cfg.APIKey = "sk-test"
	}
	if cfg.Retry == nil {
		cfg.Retry = &fastRetry
	}
	c, err := New(cfg)
	must.NoError(t, err)
	return c, rec
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestSystemOneRequestAndResponse(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TypeSafe-Request-Id", "req_1")
		writeJSON(w, 200, noulResponse)
	})

	res, err := c.SystemOne(t.Context(), Request{
		State:     "Help! My payouts have been failing for 3 days.",
		Questions: map[string]Question{"is_urgent": Noul("Does this convey urgency?")},
	})
	must.NoError(t, err)
	must.Eq(t, "req_1", res.RequestID)
	must.Eq(t, "jev-1.13.0", res.Model)
	must.Eq(t, Answer(NoulAnswer{Noul: 0.95}), res.Answers["is_urgent"])
	must.Eq(t, Usage{InputTokens: 307, OutputTokens: 20}, res.Usage)

	r := rec.last.Load()
	must.Eq(t, http.MethodPost, r.Method)
	must.Eq(t, "/v1/systemone", r.URL.Path)
	must.Eq(t, "Bearer sk-test", r.Header.Get("Authorization"))
	must.Eq(t, "application/json", r.Header.Get("Accept"))
	must.Eq(t, "application/json", r.Header.Get("Content-Type"))
	must.Eq(t, "typesafe-go/"+Version, r.Header.Get("User-Agent"))
	must.Eq(t, "typesafe-go/"+Version, r.Header.Get("X-TypeSafe-SDK"))
	must.StrHasPrefix(t, "go/go", r.Header.Get("X-TypeSafe-Runtime"))
	must.Eq(t, "", r.Header.Get("X-TypeSafe-Retry-Count"), must.Sprint("no retry header on the first attempt"))

	// Request body example from https://docs.typesafe.ai/api.md.
	var got, want bytes.Buffer
	must.NoError(t, json.Compact(&got, *rec.lastBody.Load()))
	must.NoError(t, json.Compact(&want, []byte(`{"state":"Help! My payouts have been failing for 3 days.","model":"jev-latest","questions":{"is_urgent":{"type":"noul","instructions":"Does this convey urgency?"}}}`)))
	must.Eq(t, want.String(), got.String())
}

func TestSystemOneNullStateAndModelOverride(t *testing.T) {
	c, rec := newServer(t, Config{Model: "jev-cfg"}, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, noulResponse)
	})
	_, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}, Model: "jev-req"})
	must.NoError(t, err)
	body := string(*rec.lastBody.Load())
	must.StrContains(t, body, `"state":null`)
	must.StrContains(t, body, `"model":"jev-req"`)

	_, err = c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.NoError(t, err)
	must.StrContains(t, string(*rec.lastBody.Load()), `"model":"jev-cfg"`)
}

func TestSystemOneValidatesBeforeSending(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, noulResponse) })

	_, err := c.SystemOne(t.Context(), Request{})
	must.EqError(t, err, "typesafe: at least one question is required")

	_, err = c.SystemOne(t.Context(), Request{Questions: map[string]Question{"s": Score("q", []any{"one"})}})
	must.EqError(t, err, `typesafe: score question "s" has 1 criteria; at least two scores are required`)

	must.Eq(t, int32(0), rec.requests.Load())
}

func TestSystemOneWarnsOnUnknownAnswerType(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, _ := newServer(t, Config{Logger: logger}, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"model":"m","answers":{"k":{"type":"vector"},"n":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1,"output_tokens":1}}`)
	})
	res, err := c.SystemOne(t.Context(), Request{State: "SECRET STATE", Questions: map[string]Question{"n": Noul("SECRET QUESTION")}})
	must.NoError(t, err)
	must.Eq(t, Answer(UnknownAnswer{Type: "vector", Raw: json.RawMessage(`{"type":"vector"}`)}), res.Answers["k"])

	out := logs.String()
	must.StrContains(t, out, "level=WARN")
	must.StrContains(t, out, "names=[k]")
	must.StrContains(t, out, "level=DEBUG")
	must.StrContains(t, out, "status=200")
	must.StrNotContains(t, out, "SECRET", must.Sprint("bodies are never logged"))
	must.StrNotContains(t, out, "sk-test", must.Sprint("the key is never logged"))
	must.StrNotContains(t, out, "Bearer")
}

func TestSystemOneDecodeError(t *testing.T) {
	c, _ := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TypeSafe-Request-Id", "req_bad")
		writeJSON(w, 200, `{"model":`)
	})
	_, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.Error(t, err)
	must.StrHasPrefix(t, "typesafe: POST /v1/systemone: decode response (request req_bad): ", err.Error())
	var syntaxErr *json.SyntaxError
	must.True(t, errors.As(err, &syntaxErr))
}

func TestModels(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"models":[{"name":"jev-latest","description":"Flagship","release_date":"2026-09-15"}]}`)
	})
	models, err := c.Models(t.Context())
	must.NoError(t, err)
	must.Eq(t, []Model{{Name: "jev-latest", Description: "Flagship", ReleaseDate: "2026-09-15"}}, models)

	r := rec.last.Load()
	must.Eq(t, http.MethodGet, r.Method)
	must.Eq(t, "/v1/models", r.URL.Path)
	must.Eq(t, "", r.Header.Get("Content-Type"), must.Sprint("no content type without a body"))
	must.Eq(t, 0, len(*rec.lastBody.Load()))
}

func TestModelsRejectsUnexpectedShape(t *testing.T) {
	c, _ := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, `{"data":[]}`) })
	_, err := c.Models(t.Context())
	must.EqError(t, err, `typesafe: GET /v1/models: unexpected response shape; expected {"models": [...]}`)
}

func TestAPIErrorNotRetriedOn400(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TypeSafe-Request-Id", "req_400")
		writeJSON(w, 400, `{"error":"bad question"}`)
	})
	_, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.EqError(t, err, "typesafe: POST /v1/systemone: 400 bad question (request req_400)")
	var apiErr *Error
	must.True(t, errors.As(err, &apiErr))
	must.Eq(t, 400, apiErr.Status)
	must.Eq(t, "req_400", apiErr.RequestID)
	must.Eq(t, int32(1), rec.requests.Load())
}

func TestRetryOn529ThenSuccess(t *testing.T) {
	var n atomic.Int32
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			writeJSON(w, 529, `{"error":"overloaded"}`)
			return
		}
		writeJSON(w, 200, noulResponse)
	})
	res, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.NoError(t, err)
	must.Eq(t, Answer(NoulAnswer{Noul: 0.95}), res.Answers["is_urgent"])
	must.Eq(t, int32(2), rec.requests.Load())
	must.Eq(t, "1", rec.last.Load().Header.Get("X-TypeSafe-Retry-Count"))
}

func TestRetryExhaustionReturnsLastError(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 429, `{"error":"slow down"}`)
	})
	_, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.True(t, errors.Is(err, ErrRateLimited))
	must.Eq(t, int32(3), rec.requests.Load(), must.Sprint("1 attempt + 2 retries"))
	must.Eq(t, "2", rec.last.Load().Header.Get("X-TypeSafe-Retry-Count"))
}

func TestRetriesDisabled(t *testing.T) {
	c, rec := newServer(t, Config{Retry: &RetryPolicy{}}, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 503, ``)
	})
	_, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	var apiErr *Error
	must.True(t, errors.As(err, &apiErr))
	must.Eq(t, 503, apiErr.Status)
	must.Eq(t, int32(1), rec.requests.Load())
}

func TestRetryLogsAtInfo(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var n atomic.Int32
	c, _ := newServer(t, Config{Logger: logger}, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			writeJSON(w, 500, `boom`)
			return
		}
		writeJSON(w, 200, noulResponse)
	})
	_, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.NoError(t, err)
	out := logs.String()
	must.StrContains(t, out, "level=INFO")
	must.StrContains(t, out, "retrying")
	must.StrContains(t, out, "reason=500")
	must.StrNotContains(t, out, "level=DEBUG", must.Sprint("attempt summaries are DEBUG only"))
}

func TestCancelDuringBackoff(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10")
		writeJSON(w, 429, ``)
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := c.SystemOne(ctx, Request{Questions: map[string]Question{"q": Noul("x")}})
		done <- err
	}()
	// Wait for the first request to land, then cancel while the client sleeps
	// on the 10s Retry-After.
	deadline := time.After(5 * time.Second)
	for rec.requests.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("first request never arrived")
		default:
			runtime.Gosched()
		}
	}
	cancel()
	select {
	case err := <-done:
		must.True(t, errors.Is(err, context.Canceled))
		must.StrHasPrefix(t, "typesafe: POST /v1/systemone: ", err.Error())
	case <-time.After(5 * time.Second):
		t.Fatal("SystemOne did not return after cancel")
	}
	must.Eq(t, int32(1), rec.requests.Load(), must.Sprint("no request after cancel"))
}

func TestAttemptTimeoutIsRetried(t *testing.T) {
	var n atomic.Int32
	c, rec := newServer(t, Config{Timeout: 50 * time.Millisecond}, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			<-r.Context().Done() // hold the first attempt until the client gives up
			return
		}
		writeJSON(w, 200, noulResponse)
	})
	res, err := c.SystemOne(t.Context(), Request{Questions: map[string]Question{"q": Noul("x")}})
	must.NoError(t, err)
	must.Eq(t, Answer(NoulAnswer{Noul: 0.95}), res.Answers["is_urgent"])
	must.Eq(t, int32(2), rec.requests.Load())
}

func TestCallerDeadlineIsNotRetried(t *testing.T) {
	c, rec := newServer(t, Config{Timeout: 10 * time.Second}, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := c.SystemOne(ctx, Request{Questions: map[string]Question{"q": Noul("x")}})
	must.True(t, errors.Is(err, context.DeadlineExceeded))
	must.Eq(t, int32(1), rec.requests.Load())
}

func TestConnectionErrorIsNotAPIError(t *testing.T) {
	c, err := New(Config{APIKey: "sk", BaseURL: "http://127.0.0.1:1", Retry: &RetryPolicy{MaxRetries: 1, BackoffInitial: time.Millisecond, BackoffMax: time.Millisecond}})
	must.NoError(t, err)
	_, err = c.Models(t.Context())
	must.Error(t, err)
	must.StrHasPrefix(t, "typesafe: GET /v1/models: ", err.Error())
	var apiErr *Error
	must.False(t, errors.As(err, &apiErr), must.Sprint("a connection failure is not an *Error"))
}

func TestOversizeBodyIsAnError(t *testing.T) {
	c, rec := newServer(t, Config{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", maxBody+1)))
	})
	_, err := c.Models(t.Context())
	must.EqError(t, err, "typesafe: GET /v1/models: response body exceeds 8 MiB")
	must.Eq(t, int32(1), rec.requests.Load(), must.Sprint("oversize is not retried"))
}
