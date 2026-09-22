package typesafe

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

func TestNewAPIError(t *testing.T) {
	h := http.Header{"X-Typesafe-Request-Id": {"req_123"}, "Retry-After-Ms": {"250"}}
	e := newAPIError(429, []byte(`{"error":"slow down"}`), h)
	must.Eq(t, 429, e.Status)
	must.Eq(t, "slow down", e.Message)
	must.Eq(t, "req_123", e.RequestID)
	must.Eq(t, 250*time.Millisecond, e.RetryAfter)
	must.Eq(t, `{"error":"slow down"}`, string(e.Body))
	must.Eq(t, "429 slow down (request req_123)", e.Error())
}

func TestErrorString(t *testing.T) {
	must.Eq(t, "500", newAPIError(500, nil, http.Header{}).Error())
	must.Eq(t, "502 upstream", newAPIError(502, []byte("upstream"), http.Header{}).Error())

	long := strings.Repeat("x", 300)
	got := newAPIError(400, []byte(long), http.Header{}).Error()
	must.Eq(t, "400 "+strings.Repeat("x", 200)+"…", got)

	longRunes := strings.Repeat("é", 300)
	got = newAPIError(400, []byte(longRunes), http.Header{}).Error()
	must.Eq(t, "400 "+strings.Repeat("é", 200)+"…", got)
}

func TestExtractMessage(t *testing.T) {
	cases := map[string]string{
		`{"error":"a"}`:              "a",
		`{"error":{"message":"b"}}`:  "b",
		`{"message":"c"}`:            "c",
		`{"detail":"d"}`:             "d",
		`{"detail":{"message":"e"}}`: "e",
		`{"detail":[{"loc":["body","questions","x","criteria"],"msg":"field required"},{"loc":["body"],"msg":"bad"}]}`: "questions.x.criteria: field required; bad",
		`{"detail":[{"nope":1}]}`: "",
		`{"unrelated":true}`:      "",
		`"just a string"`:         "just a string",
		`plain text`:              "plain text",
		``:                        "",
		`   `:                     "",
	}
	for body, want := range cases {
		must.Eq(t, want, extractMessage([]byte(body)), must.Sprintf("body %q", body))
	}
}

func TestErrorIs(t *testing.T) {
	must.True(t, errors.Is(newAPIError(401, nil, http.Header{}), ErrAuthentication))
	must.True(t, errors.Is(newAPIError(429, nil, http.Header{}), ErrRateLimited))
	must.True(t, errors.Is(newAPIError(529, nil, http.Header{}), ErrOverloaded))
	must.False(t, errors.Is(newAPIError(500, nil, http.Header{}), ErrOverloaded))
	must.False(t, errors.Is(newAPIError(403, nil, http.Header{}), ErrAuthentication))

	wrappedRateLimit := fmt.Errorf("typesafe: POST /v1/systemone: %w", newAPIError(429, nil, http.Header{}))
	must.True(t, errors.Is(wrappedRateLimit, ErrRateLimited))

	var apiErr *Error
	wrapped := errors.Join(errors.New("outer"), newAPIError(404, nil, http.Header{}))
	must.True(t, errors.As(wrapped, &apiErr))
	must.Eq(t, 404, apiErr.Status)
}
