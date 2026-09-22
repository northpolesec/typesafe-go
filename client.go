package typesafe

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
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
