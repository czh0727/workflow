package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"git.sotatts.online/matrix/matrix/packages/backend/go/obs"
)

// MaxResponseBody is the default per-response body cap. Callers that expect
// large bodies (e.g. inference clients returning base64 audio) should override
// it with WithMaxBodyBytes.
const MaxResponseBody = 1 << 20 // 1 MiB
const defaultRetryInitialInterval = 50 * time.Millisecond

type Option func(*Client)

// RetryPolicy configures the retry count and backoff used for retryable
// requests.
type RetryPolicy struct {
	Times           int
	InitialInterval time.Duration
}

func WithMaxBodyBytes(n int64) Option {
	return func(c *Client) {
		c.MaxBodyBytes = n
	}
}

func WithRetryTimes(n int) Option {
	return func(c *Client) {
		c.retryTimes = n
	}
}

// Client is a thin wrapper around *http.Client with JSON helpers and structured logging.
type Client struct {
	// BaseURL is kept for compatibility with existing struct field users.
	// New code should call BaseURLValue so hot reloads are visible safely.
	BaseURL        string
	HTTPClient     *http.Client
	Module         string
	DefaultHeaders map[string]string
	ContextHeaders func(context.Context) map[string]string
	// MaxBodyBytes caps the response body read. 0 means use the package default
	// MaxResponseBody. Set a negative value for callers that need unlimited reads.
	MaxBodyBytes int64
	// RetryTimes retries GET/HEAD requests after retryable transport errors or
	// retryable HTTP statuses. 0 keeps the default single attempt.
	retryTimes  int
	middlewares []Middleware
	handler     HandlerFunc
	log         *obs.Logger
	baseURL     atomic.Value
}

// New creates a Client. module is used as the logger module name (e.g. "iam", "metering").
func New(baseURL string, httpClient *http.Client, module string, opts ...Option) *Client {
	c := &Client{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
		Module:     module,
		log:        obs.WithModule(module),
	}
	c.handler = c.sendRequest
	c.baseURL.Store(baseURL)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetBaseURL updates the upstream endpoint used by subsequent requests.
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL.Store(baseURL)
}

// WithMaxBodyBytes updates the response body cap and returns c for existing
// chained callers. Prefer passing httpclient.WithMaxBodyBytes to New code.
func (c *Client) WithMaxBodyBytes(n int64) *Client {
	c.MaxBodyBytes = n
	return c
}

// BaseURLValue returns the currently active upstream endpoint.
func (c *Client) BaseURLValue() string {
	if v := c.baseURL.Load(); v != nil {
		return v.(string)
	}
	return c.BaseURL
}

// Response bundles status code and body bytes from an HTTP call.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// GetJSON sends a GET and returns the raw response.
func (c *Client) GetJSON(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodGet, path, nil, opts...)
}

// PostJSON marshals payload to JSON and sends a POST.
func (c *Client) PostJSON(ctx context.Context, path string, payload any, opts ...RequestOption) (*Response, error) {
	return c.SendJSON(ctx, http.MethodPost, path, payload, opts...)
}

// PutJSON marshals payload to JSON and sends a PUT.
func (c *Client) PutJSON(ctx context.Context, path string, payload any, opts ...RequestOption) (*Response, error) {
	return c.SendJSON(ctx, http.MethodPut, path, payload, opts...)
}

// DeleteJSON sends a DELETE and returns the raw response.
func (c *Client) DeleteJSON(ctx context.Context, path string, opts ...RequestOption) (*Response, error) {
	return c.Do(ctx, http.MethodDelete, path, nil, opts...)
}

// SendJSON marshals payload and sends it with a replayable body.
func (c *Client) SendJSON(ctx context.Context, method, path string, payload any, opts ...RequestOption) (*Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", c.logPrefix(), err)
	}
	req := &Request{
		Method: method,
		Path:   path,
		Body:   bytes.NewReader(data),
		GetBody: func() (io.Reader, error) {
			return bytes.NewReader(data), nil
		},
		Header: make(http.Header),
	}
	for _, opt := range opts {
		opt(req)
	}
	return c.handler(ctx, req)
}

// Do executes an HTTP request against BaseURL+path. body may be nil.
func (c *Client) Do(ctx context.Context, method, path string, body io.Reader, opts ...RequestOption) (*Response, error) {
	req := &Request{
		Method: method,
		Path:   path,
		Body:   body,
		Header: make(http.Header),
	}
	for _, opt := range opts {
		opt(req)
	}
	// handler starts as sendRequest and is replaced by Use with the middleware chain.
	return c.handler(ctx, req)
}

func (c *Client) sendRequest(ctx context.Context, r *Request) (*Response, error) {
	url := c.BaseURLValue() + r.Path
	req, err := http.NewRequestWithContext(ctx, r.Method, url, r.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", c.logPrefix(), err)
	}
	if r.GetBody != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := r.GetBody()
			if err != nil {
				return nil, err
			}
			return io.NopCloser(body), nil
		}
	}
	for k, v := range c.DefaultHeaders {
		req.Header.Set(k, v)
	}
	if c.ContextHeaders != nil {
		for k, v := range c.ContextHeaders(ctx) {
			if v != "" {
				req.Header.Set(k, v)
			}
		}
	}
	for k, values := range r.Header {
		req.Header.Del(k)
		for _, v := range values {
			if v != "" {
				req.Header.Add(k, v)
			}
		}
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	policy := RetryPolicy{
		InitialInterval: defaultRetryInitialInterval,
	}
	if r.retryPolicy != nil {
		policy = *r.retryPolicy
	} else if r.Method == http.MethodGet || r.Method == http.MethodHead {
		policy.Times = c.retryTimes
	}
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		attemptReq := req
		if attempt > 0 && req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				err = bodyErr
				break
			}
			attemptReq = req.Clone(req.Context())
			attemptReq.Body = body
			attemptReq.GetBody = req.GetBody
		}

		resp, err = c.HTTPClient.Do(attemptReq)
		if err != nil {
			if attempt >= policy.Times || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
		} else {
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < http.StatusInternalServerError {
				break
			}
			if attempt >= policy.Times {
				break
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxResponseBody))
			resp.Body.Close()
		}

		time.Sleep(time.Duration(attempt+1) * policy.InitialInterval)
	}

	latency := time.Since(start)
	if err != nil {
		c.log.Error("request failed",
			"method", r.Method, "url", url,
			"error", err, "latency_ms", latency.Milliseconds(),
		)
		return nil, fmt.Errorf("%s: %s %s: %w", c.logPrefix(), r.Method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := c.readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	c.log.Info("request",
		"method", r.Method, "url", url,
		"status", resp.StatusCode, "latency_ms", latency.Milliseconds(),
	)

	return &Response{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: respBody}, nil
}

// DecodeJSON unmarshals the response body into dest.
func (r *Response) DecodeJSON(dest any) error {
	return json.Unmarshal(r.Body, dest)
}

// ErrorResponse is the common error envelope used across internal services.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Status  string `json:"status,omitempty"`
}

// ToError converts a non-OK response into a structured Go error.
// It tries to parse the body as ErrorResponse first.
func (r *Response) ToError(prefix string) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(r.Body, &errResp); err == nil && errResp.Error != "" {
		return fmt.Errorf("%s: %d %s: %s", prefix, r.StatusCode, errResp.Error, errResp.Message)
	}
	return fmt.Errorf("%s: unexpected status %d: %s", prefix, r.StatusCode, TruncateBody(r.Body, 512))
}

// TruncateBody returns at most max bytes of b as a string.
func TruncateBody(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

func (c *Client) logPrefix() string {
	return c.Module
}

func (c *Client) readResponseBody(body io.Reader) ([]byte, error) {
	maxBody := c.MaxBodyBytes
	if maxBody < 0 {
		respBody, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("%s: read response: %w", c.logPrefix(), err)
		}
		return respBody, nil
	}
	if maxBody == 0 {
		maxBody = MaxResponseBody
	}

	respBody, err := io.ReadAll(io.LimitReader(body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", c.logPrefix(), err)
	}
	// Detect truncation: if we hit the cap exactly AND there is more data
	// upstream, we silently dropped bytes which would yield bogus JSON errors
	// downstream. Surface a clear error instead.
	if int64(len(respBody)) == maxBody {
		var probe [1]byte
		if n, _ := body.Read(probe[:]); n > 0 {
			return nil, fmt.Errorf("%s: response body exceeded cap of %d bytes (set WithMaxBodyBytes higher)", c.logPrefix(), maxBody)
		}
	}
	return respBody, nil
}
