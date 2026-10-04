package sse

import (
	"fmt"
	"io"
	"mime"
	"net/http"
)

const maxErrorBodyBytes int64 = 1 << 20

// Client opens Server-Sent Events streams with an injected HTTP client.
type Client struct {
	http *http.Client
}

func New(httpClient *http.Client) *Client {
	return &Client{http: httpClient}
}

// HTTPError is returned when the server rejects a request before opening a stream.
type HTTPError struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("SSE request returned HTTP status %d", e.StatusCode)
}

// Open sends req and returns a stream for a successful text/event-stream response.
// The caller must close the returned stream.
func (c *Client) Open(req *http.Request) (*Stream, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("SSE HTTP client is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("SSE request is nil")
	}

	req = req.Clone(req.Context())
	req.Header = req.Header.Clone()
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send SSE request: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		if readErr != nil {
			return nil, fmt.Errorf("read SSE error response: %w", readErr)
		}
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Header:     resp.Header.Clone(),
			Body:       body,
		}
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected SSE content type %q", resp.Header.Get("Content-Type"))
	}
	return newStream(resp.Header, resp.Body), nil
}
