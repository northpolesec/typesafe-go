package typesafe

import (
	"net/http"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

func TestDefaultRetryPolicy(t *testing.T) {
	must.Eq(t, RetryPolicy{MaxRetries: 2, BackoffInitial: 500 * time.Millisecond, BackoffMax: 5 * time.Second}, DefaultRetryPolicy())
}

func TestRetryableStatus(t *testing.T) {
	for _, s := range []int{408, 429, 500, 502, 529, 599} {
		must.True(t, retryableStatus(s), must.Sprintf("status %d", s))
	}
	for _, s := range []int{200, 400, 401, 403, 404, 422, 600} {
		must.False(t, retryableStatus(s), must.Sprintf("status %d", s))
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		header http.Header
		want   time.Duration
		ok     bool
	}{
		{"absent", http.Header{}, 0, false},
		{"ms", http.Header{"Retry-After-Ms": {"100"}}, 100 * time.Millisecond, true},
		{"ms zero", http.Header{"Retry-After-Ms": {"0"}}, 0, true},
		{"ms invalid falls through to seconds", http.Header{"Retry-After-Ms": {"abc"}, "Retry-After": {"2"}}, 2 * time.Second, true},
		{"ms negative ignored", http.Header{"Retry-After-Ms": {"-5"}}, 0, false},
		{"seconds", http.Header{"Retry-After": {"120"}}, 120 * time.Second, true},
		{"fractional seconds", http.Header{"Retry-After": {"1.5"}}, 1500 * time.Millisecond, true},
		{"negative seconds ignored", http.Header{"Retry-After": {"-1"}}, 0, false},
		{"http date", http.Header{"Retry-After": {now.Add(30 * time.Second).Format(http.TimeFormat)}}, 30 * time.Second, true},
		{"http date in past clamps to zero", http.Header{"Retry-After": {now.Add(-30 * time.Second).Format(http.TimeFormat)}}, 0, true},
		{"garbage", http.Header{"Retry-After": {"soon"}}, 0, false},
		{"nan ignored", http.Header{"Retry-After": {"NaN"}}, 0, false},
		{"inf ignored", http.Header{"Retry-After": {"Inf"}}, 0, false},
		{"ms overflow ignored", http.Header{"Retry-After-Ms": {"9223372036854775807"}}, 0, false},
		{"seconds overflow ignored", http.Header{"Retry-After": {"1e300"}}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tc.header, now)
			must.Eq(t, tc.ok, ok)
			must.Eq(t, tc.want, got)
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	now := time.Now()
	p := DefaultRetryPolicy()

	// No header: capped exponential backoff with jitter r in [0,1).
	must.Eq(t, 500*time.Millisecond, backoffDelay(0, nil, p, 0, now))
	must.Eq(t, 1000*time.Millisecond, backoffDelay(1, nil, p, 0, now))
	must.Eq(t, 2000*time.Millisecond, backoffDelay(2, nil, p, 0, now))
	must.Eq(t, 4000*time.Millisecond, backoffDelay(3, nil, p, 0, now))
	must.Eq(t, 5000*time.Millisecond, backoffDelay(4, nil, p, 0, now), must.Sprint("capped at BackoffMax"))
	must.Eq(t, 5000*time.Millisecond, backoffDelay(60, nil, p, 0, now), must.Sprint("large n does not overflow"))
	must.Eq(t, 375*time.Millisecond, backoffDelay(0, nil, p, 1, now), must.Sprint("full jitter subtracts 25%"))
	must.Eq(t, time.Duration(0), backoffDelay(3, nil, RetryPolicy{MaxRetries: 1}, 0.5, now), must.Sprint("zero backoff is literal"))

	// Server delay wins when present and within the cap.
	h := http.Header{"Retry-After-Ms": {"100"}}
	must.Eq(t, 100*time.Millisecond, backoffDelay(0, h, p, 0.9, now))
	must.Eq(t, 60*time.Second, backoffDelay(0, http.Header{"Retry-After": {"60"}}, p, 0.9, now))

	// Over the cap or unparseable: fall back to backoff.
	must.Eq(t, 500*time.Millisecond, backoffDelay(0, http.Header{"Retry-After": {"120"}}, p, 0, now))
	must.Eq(t, 500*time.Millisecond, backoffDelay(0, http.Header{"Retry-After": {"soon"}}, p, 0, now))

	// HTTP-date Retry-After: honored within the cap, backoff beyond it.
	nowSec := now.UTC().Truncate(time.Second)
	must.Eq(t, 30*time.Second, backoffDelay(0, http.Header{"Retry-After": {nowSec.Add(30 * time.Second).Format(http.TimeFormat)}}, p, 0.9, nowSec))
	must.Eq(t, 500*time.Millisecond, backoffDelay(0, http.Header{"Retry-After": {nowSec.Add(2 * time.Minute).Format(http.TimeFormat)}}, p, 0, nowSec))
}
