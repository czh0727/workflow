package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type trackingBody struct {
	io.Reader
	readCount  int
	closeCount int
}

func (b *trackingBody) Read(p []byte) (int, error) {
	b.readCount++
	return b.Reader.Read(p)
}

func (b *trackingBody) Close() error {
	b.closeCount++
	return nil
}

func TestClientOpenReturnsUnreadStream(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader("data: hello\n\n")}
	client := New(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("Accept"); got != "text/event-stream" {
				t.Fatalf("Accept = %q, want text/event-stream", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
				Body:       body,
				Request:    req,
			}, nil
		}),
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.test/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	stream, err := client.Open(req)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if body.readCount != 0 {
		t.Fatalf("response body read count = %d, want 0", body.readCount)
	}
	if body.closeCount != 0 {
		t.Fatal("response body was closed before the caller consumed it")
	}
	if req.Header.Get("Accept") != "" {
		t.Fatal("Open mutated the caller's request headers")
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if string(event.Data) != "hello" {
		t.Fatalf("event data = %q, want hello", event.Data)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if body.closeCount != 1 {
		t.Fatalf("response body close count = %d, want 1", body.closeCount)
	}
}

func TestClientOpenReturnsHTTPErrorAndClosesBody(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader(`{"error":"bad request"}`)}
	client := New(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
				Request:    req,
			}, nil
		}),
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.test/events", nil)

	stream, err := client.Open(req)
	if stream != nil {
		t.Fatalf("Open() stream = %+v, want nil", stream)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("Open() error = %v, want HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest || string(httpErr.Body) != `{"error":"bad request"}` {
		t.Fatalf("HTTPError = %+v", httpErr)
	}
	if body.readCount == 0 || body.closeCount != 1 {
		t.Fatalf("body reads=%d closes=%d, want read and one close", body.readCount, body.closeCount)
	}
}

func TestClientOpenRejectsUnexpectedContentType(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader(`{"status":"ok"}`)}
	client := New(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
				Request:    req,
			}, nil
		}),
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.test/events", nil)

	stream, err := client.Open(req)
	if err == nil || stream != nil {
		t.Fatalf("Open() = (%+v, %v), want nil stream and error", stream, err)
	}
	if body.closeCount != 1 {
		t.Fatalf("body close count = %d, want 1", body.closeCount)
	}
}

func TestStreamRecvParsesSSEFieldsAndLineEndings(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader(
		": comment\r\n" +
			"id: 42\r" +
			"event: update\n" +
			"retry: 1500\r\n" +
			"data: first\r\n" +
			"data: second\n\n" +
			"data:\n\n",
	)}
	stream := newStream(http.Header{"X-Test": []string{"value"}}, body)

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("first Recv() error = %v", err)
	}
	if first.ID != "42" || first.Name != "update" || string(first.Data) != "first\nsecond" || first.Retry != 1500*time.Millisecond {
		t.Fatalf("first event = %+v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("second Recv() error = %v", err)
	}
	if second.ID != "42" || second.Name != "" || len(second.Data) != 0 {
		t.Fatalf("second event = %+v", second)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("final Recv() error = %v, want EOF", err)
	}
	if got := stream.Header().Get("X-Test"); got != "value" {
		t.Fatalf("Header X-Test = %q", got)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if body.closeCount != 1 {
		t.Fatalf("body close count = %d, want 1", body.closeCount)
	}
}
