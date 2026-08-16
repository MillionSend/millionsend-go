package millionsend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// mockServer spins up an httptest server that records the last request and
// replies with status/respBody, and returns a Client pointed at it.
func mockServer(t *testing.T, status int, respBody string) (*Client, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.RawQuery = r.URL.RawQuery
		captured.Header = r.Header.Clone()
		captured.Body = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("ms_test")
	c.BaseURL = srv.URL
	return c, captured
}

// bodyMap decodes the captured request body as a JSON object.
func bodyMap(t *testing.T, rec *capturedRequest) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(rec.Body, &m))
	return m
}

func sampleEmail() *SendEmailRequest {
	return &SendEmailRequest{From: "a@x.dev", To: []string{"b@x.dev"}, Subject: "s", Text: "t"}
}

func TestNewClientDefaultsBaseURL(t *testing.T) {
	t.Setenv("MILLIONSEND_BASE_URL", "")
	c := NewClient("ms_test")
	assert.Equal(t, "http://localhost:3001", c.BaseURL)
}

func TestNewClientEnvFallback(t *testing.T) {
	t.Setenv("MILLIONSEND_API_KEY", "ms_env")
	t.Setenv("MILLIONSEND_BASE_URL", "https://mail.example.com")
	c := NewClient("")
	assert.Equal(t, "https://mail.example.com", c.BaseURL)
}

func TestTrailingSlashStripped(t *testing.T) {
	c, rec := mockServer(t, 200, `{}`)
	c.BaseURL = c.BaseURL + "/"
	_, err := c.Emails.Get("e1")
	require.NoError(t, err)
	assert.Equal(t, "/emails/e1", rec.Path)
}

func TestRequestHeaders(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"abc"}`)
	_, err := c.Emails.Send(sampleEmail())
	require.NoError(t, err)
	assert.Equal(t, "Bearer ms_test", rec.Header.Get("Authorization"))
	assert.Equal(t, "application/json", rec.Header.Get("Accept"))
	assert.Equal(t, "application/json", rec.Header.Get("Content-Type"))
	assert.Contains(t, rec.Header.Get("User-Agent"), "millionsend-go/")
}

func TestCamelToSnakeMappingOmitsEmpty(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"abc"}`)
	_, err := c.Emails.Send(&SendEmailRequest{
		From:        "a@x.dev",
		To:          []string{"b@x.dev"},
		Subject:     "s",
		Html:        "<p>h</p>",
		ReplyTo:     "r@x.dev",
		ScheduledAt: "2999-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	b := bodyMap(t, rec)
	assert.Equal(t, "r@x.dev", b["reply_to"])
	assert.Equal(t, "2999-01-01T00:00:00Z", b["scheduled_at"])
	_, hasText := b["text"]
	assert.False(t, hasText, "empty optional should be omitted")
	_, hasCc := b["cc"]
	assert.False(t, hasCc)
}

func TestIdempotencyKeyPostOnly(t *testing.T) {
	c, rec := mockServer(t, 200, `{"id":"abc"}`)
	_, err := c.Emails.Send(sampleEmail(), WithIdempotencyKey("key-123"))
	require.NoError(t, err)
	assert.Equal(t, "key-123", rec.Header.Get("Idempotency-Key"))

	// A GET carries no idempotency header (it is wired to POST sends only).
	_, err = c.Emails.Get("e1")
	require.NoError(t, err)
	assert.Empty(t, rec.Header.Get("Idempotency-Key"))
}

func TestSuccessDecodes(t *testing.T) {
	c, _ := mockServer(t, 200, `{"id":"abc"}`)
	res, err := c.Emails.Send(sampleEmail())
	require.NoError(t, err)
	assert.Equal(t, "abc", res.Id)
}

func TestErrorParsing(t *testing.T) {
	c, _ := mockServer(t, 422, `{"statusCode":422,"name":"validation_error","message":"bad"}`)
	_, err := c.Emails.Send(sampleEmail())
	require.Error(t, err)
	var mse *MillionSendError
	require.ErrorAs(t, err, &mse)
	assert.Equal(t, 422, mse.StatusCode)
	assert.Equal(t, "validation_error", mse.Name)
	assert.Equal(t, "bad", mse.Message)
}

func TestErrorFallbackForNonCanonicalBody(t *testing.T) {
	c, _ := mockServer(t, 500, `gateway boom`)
	_, err := c.Emails.Get("e1")
	require.Error(t, err)
	var mse *MillionSendError
	require.ErrorAs(t, err, &mse)
	assert.Equal(t, 500, mse.StatusCode)
	assert.Equal(t, "application_error", mse.Name)
}

func TestTransportErrorHasNullStatus(t *testing.T) {
	c := NewClient("ms_test")
	c.BaseURL = "http://127.0.0.1:1" // nothing listening: connection refused
	_, err := c.Emails.Send(sampleEmail())
	require.Error(t, err)
	var mse *MillionSendError
	require.ErrorAs(t, err, &mse)
	assert.Equal(t, 0, mse.StatusCode, "transport failures use statusCode null (0)")
}
