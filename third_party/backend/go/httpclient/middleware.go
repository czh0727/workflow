package httpclient

import (
	"context"
	"io"
	"net/http"
)

// Request is the request data passed through client middleware.
type Request struct {
	Method      string
	Path        string
	Body        io.Reader
	GetBody     func() (io.Reader, error)
	Header      http.Header
	retryPolicy *RetryPolicy
}

// RequestOption customizes a single request without changing the client default.
type RequestOption func(*Request)

// WithRequestRetryPolicy applies retry behavior to a single request.
func WithRequestRetryPolicy(policy RetryPolicy) RequestOption {
	return func(r *Request) {
		if policy.InitialInterval <= 0 {
			policy.InitialInterval = defaultRetryInitialInterval
		}
		r.retryPolicy = &policy
	}
}

// HandlerFunc sends one request and returns the buffered response.
type HandlerFunc func(ctx context.Context, req *Request) (*Response, error)

// Middleware wraps a HandlerFunc to run logic before or after the HTTP request.
type Middleware func(HandlerFunc) HandlerFunc

// Use appends middleware to the client. Call it during client setup, before
// the client is used concurrently.
func (c *Client) Use(mw ...Middleware) *Client {
	c.middlewares = append(c.middlewares, mw...)
	c.handler = c.combineHandlers(c.sendRequest)
	return c
}

func (c *Client) combineHandlers(final HandlerFunc) HandlerFunc {
	h := final
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		hf := c.middlewares[i]
		h = hf(h)
	}
	return h
}
