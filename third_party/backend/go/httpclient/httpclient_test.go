package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

type testContextHeaderKey struct{}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientSetBaseURLUpdatesRequests(t *testing.T) {
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"server":"old"}`))
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"server":"new"}`))
	}))
	defer newServer.Close()

	client := New(oldServer.URL, oldServer.Client(), "test")

	resp, err := client.GetJSON(context.Background(), "/status")
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	if got := string(resp.Body); got != `{"server":"old"}` {
		t.Fatalf("initial request body = %s", got)
	}

	client.SetBaseURL(newServer.URL)

	resp, err = client.GetJSON(context.Background(), "/status")
	if err != nil {
		t.Fatalf("reloaded request failed: %v", err)
	}
	if got := string(resp.Body); got != `{"server":"new"}` {
		t.Fatalf("reloaded request body = %s", got)
	}
}

func TestClientContextHeadersOverrideDefaultHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-App-ID"); got != "studio" {
			t.Fatalf("X-App-ID=%q, want studio", got)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test")
	client.DefaultHeaders = map[string]string{"X-App-ID": "admin"}
	client.ContextHeaders = func(ctx context.Context) map[string]string {
		appID, _ := ctx.Value(testContextHeaderKey{}).(string)
		if appID == "" {
			return nil
		}
		return map[string]string{"X-App-ID": appID}
	}

	ctx := context.WithValue(context.Background(), testContextHeaderKey{}, "studio")
	if _, err := client.GetJSON(ctx, "/status"); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

func TestClientMaxBodyBytesCompatibility(t *testing.T) {
	client := New("http://example.test", http.DefaultClient, "test", WithMaxBodyBytes(64))
	if client.MaxBodyBytes != 64 {
		t.Fatalf("MaxBodyBytes = %d, want 64", client.MaxBodyBytes)
	}

	if got := client.WithMaxBodyBytes(128); got != client {
		t.Fatal("WithMaxBodyBytes did not return receiver")
	}
	if client.MaxBodyBytes != 128 {
		t.Fatalf("MaxBodyBytes = %d, want 128", client.MaxBodyBytes)
	}
}

func TestRequestRetryPolicyRetriesRequest(t *testing.T) {
	postAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postAttempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`failed`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test")
	retryPolicy := WithRequestRetryPolicy(RetryPolicy{
		Times:           2,
		InitialInterval: time.Nanosecond,
	})

	resp, err := client.Do(context.Background(), http.MethodPost, "/status", nil, retryPolicy)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if postAttempts != 3 {
		t.Fatalf("POST attempts = %d, want 3", postAttempts)
	}
}

func TestClientRetriesReplayableJSONPost(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if got := string(body); got != `{"hello":"world"}` {
			t.Fatalf("attempt %d body = %q, want JSON payload", attempts, got)
		}
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`failed`))
			return
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test")
	postRetryPolicy := WithRequestRetryPolicy(RetryPolicy{
		Times:           2,
		InitialInterval: time.Nanosecond,
	})

	resp, err := client.SendJSON(context.Background(), http.MethodPost, "/status", map[string]string{"hello": "world"}, postRetryPolicy)
	if err != nil {
		t.Fatalf("SendJSON() error = %v", err)
	}
	if got := string(resp.Body); got != `ok` {
		t.Fatalf("body = %q, want ok", got)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestClientUseAppliesMiddlewareInOrderForEachRequest(t *testing.T) {
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		events = append(events, "send:"+r.Header.Get("X-Flow"))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	middleware := func(name string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, req *Request) (*Response, error) {
				events = append(events, name+":before")
				req.Header.Add("X-Flow", name)
				resp, err := next(ctx, req)
				events = append(events, name+":after")
				return resp, err
			}
		}
	}

	client := New(server.URL, server.Client(), "test").Use(middleware("outer"), middleware("inner"))
	for i := 0; i < 2; i++ {
		if _, err := client.GetJSON(context.Background(), "/status"); err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
	}

	want := []string{
		"outer:before", "inner:before", "send:outer", "inner:after", "outer:after",
		"outer:before", "inner:before", "send:outer", "inner:after", "outer:after",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("middleware events = %#v, want %#v", events, want)
	}
}

func TestClientDoesNotRetryByDefault(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`failed`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test")
	resp, err := client.GetJSON(context.Background(), "/status")
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestClientRetriesGetRetryableStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`failed`))
			return
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test", WithRetryTimes(2))

	resp, err := client.GetJSON(context.Background(), "/status")
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if got := string(resp.Body); got != `ok` {
		t.Fatalf("body = %q, want ok", got)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestClientRetriesGetTransportError(t *testing.T) {
	attempts := 0
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("temporary failure")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`ok`)),
				Request:    req,
			}, nil
		}),
	}

	client := New("http://example.test", httpClient, "test", WithRetryTimes(1))

	resp, err := client.GetJSON(context.Background(), "/status")
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if got := string(resp.Body); got != `ok` {
		t.Fatalf("body = %q, want ok", got)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestClientRetriesGetTransportErrorWithRequestRetryPolicy(t *testing.T) {
	attempts := 0
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("temporary failure")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`ok`)),
				Request:    req,
			}, nil
		}),
	}

	client := New("http://example.test", httpClient, "test")
	retryPolicy := WithRequestRetryPolicy(RetryPolicy{
		Times:           1,
		InitialInterval: time.Nanosecond,
	})

	resp, err := client.GetJSON(context.Background(), "/status", retryPolicy)
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if got := string(resp.Body); got != `ok` {
		t.Fatalf("body = %q, want ok", got)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRequestRetryPolicyRetriesRequestWithBody(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`failed`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test")
	retryPolicy := WithRequestRetryPolicy(RetryPolicy{
		Times:           2,
		InitialInterval: time.Nanosecond,
	})

	resp, err := client.Do(context.Background(), http.MethodGet, "/status", strings.NewReader(`{"query":"value"}`), retryPolicy)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestClientDoesNotRetryPost(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`failed`))
	}))
	defer server.Close()

	client := New(server.URL, server.Client(), "test", WithRetryTimes(2))

	resp, err := client.PostJSON(context.Background(), "/status", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
