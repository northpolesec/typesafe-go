package typesafe

import (
	"net/http"
	"strconv"
	"time"
)

const (
	defaultMaxRetries     = 2
	defaultBackoffInitial = 500 * time.Millisecond
	defaultBackoffMax     = 5 * time.Second
	// backoffJitter is the fraction of each backoff delay randomly subtracted.
	backoffJitter = 0.25
	// maxRetryAfter caps a server-requested delay; longer values fall back to backoff.
	maxRetryAfter = 60 * time.Second
	// statusOverloaded is TypeSafe's non-standard "temporarily overloaded" status.
	statusOverloaded = 529
)

// RetryPolicy controls retries after a failed attempt. Fields are literal: a
// zero field means zero, not the default. Start from DefaultRetryPolicy to
// change one field, or pass &RetryPolicy{} to Config.Retry to disable retries.
type RetryPolicy struct {
	// MaxRetries is the number of retries after the first attempt; 0 disables retries.
	MaxRetries int
	// BackoffInitial is the first backoff delay, doubled on each retry.
	BackoffInitial time.Duration
	// BackoffMax caps the doubled delay.
	BackoffMax time.Duration
}

// DefaultRetryPolicy returns the SDK defaults: 2 retries, 500ms initial
// backoff doubling to a 5s cap.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     defaultMaxRetries,
		BackoffInitial: defaultBackoffInitial,
		BackoffMax:     defaultBackoffMax,
	}
}

// retryableStatus reports whether a response status is retried: 408, 429, and 5xx.
func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		(status >= 500 && status <= 599)
}

// parseRetryAfter reads the server-requested delay from retry-after-ms
// (preferred) or Retry-After (delay-seconds or an HTTP-date). ok is false
// when neither header carries a valid, non-negative delay.
func parseRetryAfter(h http.Header, now time.Time) (d time.Duration, ok bool) {
	if v := h.Get("Retry-After-Ms"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms >= 0 {
			return time.Duration(ms) * time.Millisecond, true
		}
	}
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs * float64(time.Second)), true
	}
	if at, err := http.ParseTime(v); err == nil {
		return max(0, at.Sub(now)), true
	}
	return 0, false
}

// backoffDelay returns the wait before zero-based retry n. A server delay at or
// under maxRetryAfter wins; otherwise the doubled BackoffInitial, capped at
// BackoffMax, minus a random fraction of up to backoffJitter. r in [0,1) is
// the random draw, passed in so the arithmetic is testable.
func backoffDelay(n int, h http.Header, p RetryPolicy, r float64, now time.Time) time.Duration {
	if h != nil {
		if d, ok := parseRetryAfter(h, now); ok && d <= maxRetryAfter {
			return d
		}
	}
	d := p.BackoffInitial
	for i := 0; i < n && d < p.BackoffMax; i++ {
		d *= 2
	}
	d = min(d, p.BackoffMax)
	return time.Duration(float64(d) * (1 - r*backoffJitter))
}
