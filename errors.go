package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	requestIDHeader = "X-TypeSafe-Request-Id"
	// maxMessageLen bounds the server message reproduced in Error().
	maxMessageLen = 200
)

// Sentinels for the statuses a caller branches on. Match them with errors.Is.
// Every other status is available through errors.As on *Error and Error.Status.
var (
	ErrAuthentication = errors.New("typesafe: authentication failed") // 401
	ErrRateLimited    = errors.New("typesafe: rate limited")          // 429
	ErrOverloaded     = errors.New("typesafe: overloaded")            // 529
)

// Error is a non-2xx response from the API, returned after any retries.
type Error struct {
	// Status is the HTTP status code.
	Status int
	// Body is the raw response body.
	Body []byte
	// Message is the human-readable message extracted from the body, or "".
	Message string
	// RequestID is the x-typesafe-request-id header, or "".
	RequestID string
	// RetryAfter is the server-requested delay from retry-after-ms or
	// Retry-After, or 0 when absent.
	RetryAfter time.Duration
}

func newAPIError(status int, body []byte, h http.Header) *Error {
	retryAfter, _ := parseRetryAfter(h, time.Now())
	return &Error{
		Status:     status,
		Body:       body,
		Message:    extractMessage(body),
		RequestID:  h.Get(requestIDHeader),
		RetryAfter: retryAfter,
	}
}

// Error returns the status, the message truncated to maxMessageLen characters,
// and the request id. It never includes the raw body.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(e.Status))
	if e.Message != "" {
		b.WriteByte(' ')
		if runes := []rune(e.Message); len(runes) > maxMessageLen {
			b.WriteString(string(runes[:maxMessageLen]))
			b.WriteString("…")
		} else {
			b.WriteString(e.Message)
		}
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request %s)", e.RequestID)
	}
	return b.String()
}

// Is maps the status to its sentinel so errors.Is(err, ErrRateLimited) works
// through any wrapping.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrAuthentication:
		return e.Status == http.StatusUnauthorized
	case ErrRateLimited:
		return e.Status == http.StatusTooManyRequests
	case ErrOverloaded:
		return e.Status == statusOverloaded
	}
	return false
}

// extractMessage pulls a message from the shapes the API and its proxies
// produce: a JSON string; {error}; {error:{message}}; {message}; {detail}
// as a string, {message}, or a validation array; or non-JSON text.
func extractMessage(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return text
	}
	switch m := v.(type) {
	case string:
		return m
	case map[string]any:
		if s, ok := m["error"].(string); ok {
			return s
		}
		if em, ok := m["error"].(map[string]any); ok {
			if s, ok := em["message"].(string); ok {
				return s
			}
		}
		if s, ok := m["message"].(string); ok {
			return s
		}
		switch d := m["detail"].(type) {
		case string:
			return d
		case map[string]any:
			if s, ok := d["message"].(string); ok {
				return s
			}
		case []any:
			return describeValidationErrors(d)
		}
	}
	return ""
}

// describeValidationErrors formats a 422 detail array as "loc.path: msg; ..."
// with the leading "body" location segment dropped.
func describeValidationErrors(items []any) string {
	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msg, ok := m["msg"].(string)
		if !ok {
			continue
		}
		var loc []string
		if segs, ok := m["loc"].([]any); ok {
			for _, seg := range segs {
				if s := fmt.Sprint(seg); s != "body" {
					loc = append(loc, s)
				}
			}
		}
		if len(loc) > 0 {
			msg = strings.Join(loc, ".") + ": " + msg
		}
		parts = append(parts, msg)
	}
	return strings.Join(parts, "; ")
}
