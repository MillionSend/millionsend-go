// Package millionsend is the official Go client for the MillionSend API — a
// self-hostable, Resend-compatible email platform.
//
// It deliberately mirrors the shape of resend-go, so migrating is close to a
// find-and-replace: swap the import and constructor, then point Client.BaseURL
// at your instance.
//
//	client := millionsend.NewClient("ms_123")
//	client.BaseURL = "https://mail.acme.dev"
//	res, err := client.Emails.Send(&millionsend.SendEmailRequest{
//		From:    "Acme <onboarding@acme.dev>",
//		To:      []string{"delivered@resend.dev"},
//		Subject: "Hello",
//		Html:    "<strong>it works</strong>",
//	})
//
// Every method returns (*T, error); on a non-2xx response the error is a
// *MillionSendError carrying the API's { statusCode, name, message }.
package millionsend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, reported in the User-Agent.
const Version = "0.3.0"

// MillionSend is self-hosted, so there is no cloud default base URL.
const defaultBaseURL = "http://localhost:3001"

// Client talks to a MillionSend instance. Construct it with NewClient and reuse
// it; it is safe for concurrent use.
type Client struct {
	// BaseURL is the instance URL (a trailing slash is ignored). Exported so it
	// can be overridden after construction, mirroring resend-go.
	BaseURL string
	// HTTPClient performs the requests; defaults to a 30s-timeout client.
	HTTPClient *http.Client
	// UserAgent is sent on every request.
	UserAgent string
	// AllowInsecureHTTP permits a plain http:// BaseURL on a non-loopback host.
	// Off by default because the API key travels as a bearer header.
	AllowInsecureHTTP bool

	apiKey string

	Emails         *EmailsService
	Batch          *BatchService
	Contacts       *ContactsService
	Topics         *TopicsService
	Broadcasts     *BroadcastsService
	Segments       *SegmentsService
	Deliverability *DeliverabilityService
}

// NewClient returns a Client authenticating with apiKey. If apiKey is empty it
// falls back to MILLIONSEND_API_KEY. BaseURL falls back to MILLIONSEND_BASE_URL,
// then http://localhost:3001.
func NewClient(apiKey string) *Client {
	if apiKey == "" {
		apiKey = os.Getenv("MILLIONSEND_API_KEY")
	}
	baseURL := os.Getenv("MILLIONSEND_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c := &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  "millionsend-go/" + Version,
		apiKey:     apiKey,
	}
	c.Emails = &EmailsService{client: c}
	c.Batch = &BatchService{client: c}
	c.Contacts = &ContactsService{client: c}
	c.Topics = &TopicsService{client: c}
	c.Broadcasts = &BroadcastsService{client: c}
	c.Segments = &SegmentsService{client: c}
	c.Deliverability = &DeliverabilityService{client: c}
	return c
}

// String keeps the API key out of %v / %+v output (logs, panics).
func (c Client) String() string {
	return fmt.Sprintf("millionsend.Client{BaseURL:%q UserAgent:%q}", c.BaseURL, c.UserAgent)
}

// GoString keeps the API key out of %#v output.
func (c Client) GoString() string { return c.String() }

// isInsecureHTTPURL reports whether raw is an http:// URL on a non-loopback
// host. Unparseable URLs are left for the transport to reject.
func isInsecureHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host != "localhost" && host != "::1" && !strings.HasPrefix(host, "127.")
}

// RequestOption configures a single request. Only Emails.Send and Batch.Send
// accept these — the idempotency key is POST-only and wired to those two.
type RequestOption func(*requestConfig)

type requestConfig struct {
	idempotencyKey string
}

// WithIdempotencyKey attaches an Idempotency-Key header to a send.
func WithIdempotencyKey(key string) RequestOption {
	return func(c *requestConfig) { c.idempotencyKey = key }
}

func buildConfig(opts []RequestOption) requestConfig {
	var cfg requestConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// ListOptions holds the keyset pagination params shared by every list endpoint.
// After and Before are mutually exclusive; a zero Limit uses the API default.
type ListOptions struct {
	Limit  int
	After  string
	Before string
}

func (o *ListOptions) values() url.Values {
	if o == nil {
		return nil
	}
	v := url.Values{}
	if o.Limit > 0 {
		v.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.After != "" {
		v.Set("after", o.After)
	}
	if o.Before != "" {
		v.Set("before", o.Before)
	}
	return v
}

// ListResponse is the { object, data, has_more } envelope common to the
// paginated list endpoints (topics are the exception — see TopicListResponse).
type ListResponse[T any] struct {
	Object  string `json:"object"`
	Data    []T    `json:"data"`
	HasMore bool   `json:"has_more"`
}

// Ptr returns a pointer to v — handy for the optional *string / *bool fields on
// update requests, e.g. Unsubscribed: millionsend.Ptr(true).
func Ptr[T any](v T) *T { return &v }

type requestParams struct {
	method         string
	path           string
	body           any
	query          url.Values
	idempotencyKey string
}

func (c *Client) do(ctx context.Context, p requestParams, out any) error {
	var bodyReader io.Reader
	hasBody := p.body != nil && p.method != http.MethodGet && p.method != http.MethodDelete
	if hasBody {
		buf, err := json.Marshal(p.body)
		if err != nil {
			return &MillionSendError{Name: "application_error", Message: err.Error()}
		}
		bodyReader = bytes.NewReader(buf)
	}

	base := strings.TrimRight(c.BaseURL, "/")
	if !c.AllowInsecureHTTP && isInsecureHTTPURL(base) {
		return &MillionSendError{
			Name:    "application_error",
			Message: "refusing to send the API key over plain http to " + base + "; use https, or set Client.AllowInsecureHTTP = true",
		}
	}
	u := base + p.path
	if len(p.query) > 0 {
		u += "?" + p.query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, p.method, u, bodyReader)
	if err != nil {
		return &MillionSendError{Name: "application_error", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	// Idempotency is POST-only on the wire; ignored on any other method.
	if p.idempotencyKey != "" && p.method == http.MethodPost {
		req.Header.Set("Idempotency-Key", p.idempotencyKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// Never reached the API: StatusCode stays 0 (the wire's "statusCode: null").
		return &MillionSendError{Name: "application_error", Message: err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &MillionSendError{Name: "application_error", Message: err.Error()}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseError(data, resp.StatusCode)
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return &MillionSendError{Name: "application_error", Message: err.Error()}
		}
	}
	return nil
}

// doJSON runs a request and decodes a 2xx body into a fresh *T.
func doJSON[T any](c *Client, ctx context.Context, p requestParams) (*T, error) {
	out := new(T)
	if err := c.do(ctx, p, out); err != nil {
		return nil, err
	}
	return out, nil
}
