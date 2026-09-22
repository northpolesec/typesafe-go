package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the API root used when Config.BaseURL and
	// TYPESAFE_BASE_URL are both unset.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel is used when Config.Model, TYPESAFE_DEFAULT_MODEL, and
	// Request.Model are all unset.
	DefaultModel = "jev-latest"

	defaultTimeout = 10 * time.Second

	envAPIKey  = "TYPESAFE_API_KEY"
	envBaseURL = "TYPESAFE_BASE_URL"
	envModel   = "TYPESAFE_DEFAULT_MODEL"

	systemOnePath = "/v1/systemone"
	modelsPath    = "/v1/models"

	// maxBody bounds a response body read; a real response is kilobytes.
	maxBody = 8 << 20
)

// defaultHTTPClient is package-owned, never http.DefaultClient, so a caller's
// changes to the global do not leak in.
var defaultHTTPClient = &http.Client{}

// HTTPDoer is satisfied by *http.Client.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Config configures New. Every field is optional except the API key, which
// may come from the environment. Explicit values win over environment
// variables (TYPESAFE_API_KEY, TYPESAFE_BASE_URL, TYPESAFE_DEFAULT_MODEL),
// which win over defaults. Environment values are trimmed; blank means unset.
type Config struct {
	// APIKey authenticates every request.
	APIKey string
	// BaseURL is the API root; trailing slashes are removed.
	BaseURL string
	// Model is used when Request.Model is empty.
	Model string
	// Timeout bounds each attempt, including reading the body. 0 means 10s.
	// A deadline on the request context bounds the whole call instead.
	Timeout time.Duration
	// Retry is the retry policy. nil means DefaultRetryPolicy();
	// &RetryPolicy{} disables retries.
	Retry *RetryPolicy
	// HTTPClient sends requests. nil means a package-owned *http.Client.
	HTTPClient HTTPDoer
	// Logger receives DEBUG attempt summaries, INFO retry notices, and WARN
	// unknown-answer notices; never bodies, headers, or the key. nil discards.
	Logger *slog.Logger
}

// Client calls the TypeSafe API. It is safe for concurrent use.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	timeout time.Duration
	retry   RetryPolicy
	http    HTTPDoer
	logger  *slog.Logger
}

// New validates cfg and returns a Client. It returns an error, never panics,
// when the API key is missing or malformed or a numeric field is negative.
func New(cfg Config) (*Client, error) {
	apiKey := strings.TrimSpace(fromCodeOrEnv(cfg.APIKey, envAPIKey))
	if apiKey == "" {
		return nil, fmt.Errorf("typesafe: no API key: pass Config.APIKey or set %s", envAPIKey)
	}
	if !printableASCII(apiKey) {
		return nil, errors.New("typesafe: API key must be printable ASCII without whitespace")
	}

	baseURL := strings.TrimRight(fromCodeOrEnv(cfg.BaseURL, envBaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if u, err := url.Parse(baseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("typesafe: Config.BaseURL %q is not an absolute http(s) URL", baseURL)
	}

	model := fromCodeOrEnv(cfg.Model, envModel)
	if model == "" {
		model = DefaultModel
	}

	if cfg.Timeout < 0 {
		return nil, errors.New("typesafe: Config.Timeout must not be negative")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	retry := DefaultRetryPolicy()
	if cfg.Retry != nil {
		retry = *cfg.Retry
		if retry.MaxRetries < 0 || retry.BackoffInitial < 0 || retry.BackoffMax < 0 {
			return nil, errors.New("typesafe: RetryPolicy fields must not be negative")
		}
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = defaultHTTPClient
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		timeout: timeout,
		retry:   retry,
		http:    httpClient,
		logger:  logger,
	}, nil
}

// fromCodeOrEnv returns the explicit value, else the trimmed environment value.
func fromCodeOrEnv(fromCode, envVar string) string {
	if fromCode != "" {
		return fromCode
	}
	return strings.TrimSpace(os.Getenv(envVar))
}

// printableASCII reports whether s is non-empty and every byte is in '!'..'~'.
// Go's transport would otherwise reject the Authorization header late, on
// the first request, with a confusing "invalid header field value".
func printableASCII(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '!' || s[i] > '~' {
			return false
		}
	}
	return true
}

// runtimeHeader identifies the Go runtime to the API, as the JS and Python
// SDKs identify theirs.
var runtimeHeader = fmt.Sprintf("go/%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)

// errResponseTooLarge is returned, without retry, for a body over maxBody.
var errResponseTooLarge = fmt.Errorf("response body exceeds %d MiB", maxBody>>20)

// Request is the input to Client.SystemOne.
type Request struct {
	// State is the content to evaluate: a string, or a map or slice that
	// marshals to a JSON object or array. nil is sent as null.
	State any
	// Questions is a non-empty map of question name to Question. Answers
	// come back under the same names.
	Questions map[string]Question
	// Model overrides Config.Model for this request when non-empty.
	Model string
}

type systemOneWire struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

// SystemOne answers req.Questions about req.State.
//
// Config.Timeout bounds each attempt. A deadline on ctx bounds the whole
// call, including retries and backoff; cancelling ctx stops the call at once
// and is never retried. Validation errors (no questions, a Score with fewer
// than two levels) are returned before any request is sent.
//
// A non-2xx response after retries is a *Error; match ErrAuthentication,
// ErrRateLimited, and ErrOverloaded with errors.Is, or inspect Error.Status.
// Connection failures and timeouts surface as the underlying net and context
// errors.
func (c *Client) SystemOne(ctx context.Context, req Request) (*Result, error) {
	if err := validateQuestions(req.Questions); err != nil {
		return nil, fmt.Errorf("typesafe: %w", err)
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	var res Result
	requestID, err := c.do(ctx, http.MethodPost, systemOnePath, systemOneWire{State: req.State, Model: model, Questions: req.Questions}, &res)
	if err != nil {
		return nil, err
	}
	res.RequestID = requestID
	if names := res.unknownAnswerNames(); len(names) > 0 {
		c.logger.WarnContext(ctx, "typesafe: ignoring answers with unknown type", "names", names)
	}
	return &res, nil
}

// Models lists the models available to the account.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var wire struct {
		Models []Model `json:"models"`
	}
	if _, err := c.do(ctx, http.MethodGet, modelsPath, nil, &wire); err != nil {
		return nil, err
	}
	if wire.Models == nil {
		return nil, fmt.Errorf(`typesafe: GET %s: unexpected response shape; expected {"models": [...]}`, modelsPath)
	}
	return wire.Models, nil
}

// do sends one API call with retries and decodes a 2xx body into out. It
// returns the request id from the final response.
func (c *Client) do(ctx context.Context, method, path string, body, out any) (string, error) {
	wrap := func(err error) error { return fmt.Errorf("typesafe: %s %s: %w", method, path, err) }

	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return "", wrap(fmt.Errorf("encode request: %w", err))
		}
	}
	endpoint := c.baseURL + path

	for attempt := 0; ; attempt++ {
		resp, respBody, err := c.attempt(ctx, method, path, endpoint, payload, attempt)

		var reason string
		switch {
		case err != nil && ctx.Err() != nil:
			// The caller cancelled or its deadline passed: never retried.
			return "", wrap(ctx.Err())
		case errors.Is(err, errResponseTooLarge):
			return "", wrap(err)
		case errors.Is(err, context.DeadlineExceeded):
			reason = "timeout"
		case err != nil:
			reason = "connection error"
		case resp.StatusCode >= 200 && resp.StatusCode <= 299:
			requestID := resp.Header.Get(requestIDHeader)
			if err := json.Unmarshal(respBody, out); err != nil {
				return "", wrap(fmt.Errorf("decode response (request %s): %w", requestID, err))
			}
			return requestID, nil
		default:
			err = newAPIError(resp.StatusCode, respBody, resp.Header)
			if !retryableStatus(resp.StatusCode) {
				return "", wrap(err)
			}
			reason = strconv.Itoa(resp.StatusCode)
		}

		if attempt >= c.retry.MaxRetries {
			return "", wrap(err)
		}
		var headers http.Header
		if resp != nil {
			headers = resp.Header
		}
		delay := backoffDelay(attempt, headers, c.retry, rand.Float64(), time.Now())
		c.logger.InfoContext(ctx, "typesafe: retrying",
			"method", method, "path", path, "attempt", attempt+1, "delay", delay, "reason", reason)
		if err := sleep(ctx, delay); err != nil {
			return "", wrap(err)
		}
	}
}

// attempt performs one HTTP round trip, including reading the body, under
// Config.Timeout. A non-nil error means no usable response; resp is non-nil
// only when err is nil.
func (c *Client) attempt(ctx context.Context, method, path, endpoint string, payload []byte, n int) (*http.Response, []byte, error) {
	actx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(actx, method, endpoint, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "typesafe-go/"+Version)
	req.Header.Set("X-TypeSafe-SDK", "typesafe-go/"+Version)
	req.Header.Set("X-TypeSafe-Runtime", runtimeHeader)
	if n > 0 {
		req.Header.Set("X-TypeSafe-Retry-Count", strconv.Itoa(n))
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.logger.DebugContext(ctx, "typesafe: attempt failed",
			"method", method, "path", path, "attempt", n, "duration", time.Since(start), "error", err)
		return nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, nil, err
	}
	c.logger.DebugContext(ctx, "typesafe: attempt",
		"method", method, "path", path, "attempt", n, "status", resp.StatusCode,
		"duration", time.Since(start), "request_id", resp.Header.Get(requestIDHeader), "body_bytes", len(respBody))
	if len(respBody) > maxBody {
		return nil, nil, errResponseTooLarge
	}
	return resp, respBody, nil
}

// sleep waits d or until ctx is done, whichever is first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
