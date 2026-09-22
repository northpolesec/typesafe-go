package typesafe

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

func TestNewResolvesDefaults(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("TYPESAFE_BASE_URL", "")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "")

	c, err := New(Config{APIKey: "sk-test"})
	must.NoError(t, err)
	must.Eq(t, "sk-test", c.apiKey)
	must.Eq(t, "https://api.typesafe.ai", c.baseURL)
	must.Eq(t, "jev-latest", c.model)
	must.Eq(t, 10*time.Second, c.timeout)
	must.Eq(t, DefaultRetryPolicy(), c.retry)
	must.True(t, c.http == defaultHTTPClient)
	must.NotNil(t, c.logger)
	must.False(t, c.logger.Enabled(t.Context(), slog.LevelError), must.Sprint("default logger discards"))
}

func TestNewReadsEnv(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "  sk-env  ")
	t.Setenv("TYPESAFE_BASE_URL", " https://example.test/// ")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", " jev-env ")

	c, err := New(Config{})
	must.NoError(t, err)
	must.Eq(t, "sk-env", c.apiKey)
	must.Eq(t, "https://example.test", c.baseURL, must.Sprint("trailing slashes stripped"))
	must.Eq(t, "jev-env", c.model)
}

func TestNewExplicitWinsOverEnv(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "sk-env")
	t.Setenv("TYPESAFE_BASE_URL", "https://env.test")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "jev-env")

	hc := &http.Client{}
	logger := slog.New(slog.DiscardHandler)
	retry := RetryPolicy{MaxRetries: 5, BackoffInitial: time.Second, BackoffMax: 2 * time.Second}
	c, err := New(Config{
		APIKey:     "sk-code",
		BaseURL:    "https://code.test/",
		Model:      "jev-code",
		Timeout:    3 * time.Second,
		Retry:      &retry,
		HTTPClient: hc,
		Logger:     logger,
	})
	must.NoError(t, err)
	must.Eq(t, "sk-code", c.apiKey)
	must.Eq(t, "https://code.test", c.baseURL)
	must.Eq(t, "jev-code", c.model)
	must.Eq(t, 3*time.Second, c.timeout)
	must.Eq(t, retry, c.retry)
	must.True(t, c.http == hc)
	must.True(t, c.logger == logger)
}

func TestNewBlankEnvIsUnset(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "   ")
	_, err := New(Config{})
	must.EqError(t, err, "typesafe: no API key: pass Config.APIKey or set TYPESAFE_API_KEY")
}

func TestNewRejectsBadConfig(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"missing key", Config{}, "typesafe: no API key: pass Config.APIKey or set TYPESAFE_API_KEY"},
		{"key with inner space", Config{APIKey: "sk 123"}, "typesafe: API key must be printable ASCII without whitespace"},
		{"key with control char", Config{APIKey: "sk\x01"}, "typesafe: API key must be printable ASCII without whitespace"},
		{"key with non-ascii", Config{APIKey: "sk-é"}, "typesafe: API key must be printable ASCII without whitespace"},
		{"negative timeout", Config{APIKey: "sk", Timeout: -time.Second}, "typesafe: Config.Timeout must not be negative"},
		{"negative retries", Config{APIKey: "sk", Retry: &RetryPolicy{MaxRetries: -1}}, "typesafe: RetryPolicy fields must not be negative"},
		{"negative backoff", Config{APIKey: "sk", Retry: &RetryPolicy{BackoffInitial: -1}}, "typesafe: RetryPolicy fields must not be negative"},
		{"bad base url", Config{APIKey: "sk", BaseURL: "://nope"}, `typesafe: Config.BaseURL "://nope" is not an absolute http(s) URL`},
		{"relative base url", Config{APIKey: "sk", BaseURL: "api.typesafe.ai"}, `typesafe: Config.BaseURL "api.typesafe.ai" is not an absolute http(s) URL`},
		{"userinfo in base url", Config{APIKey: "sk", BaseURL: "https://user:pw@host.test"}, `typesafe: Config.BaseURL "https://user:pw@host.test" is not an absolute http(s) URL`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			must.EqError(t, err, tc.want)
		})
	}
}

func TestNewDisableRetries(t *testing.T) {
	c, err := New(Config{APIKey: "sk", Retry: &RetryPolicy{}})
	must.NoError(t, err)
	must.Eq(t, RetryPolicy{}, c.retry)
}
