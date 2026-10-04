package obs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/baggage"
)

func TestLaneHeaderRoundTripper_WritesXLane(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get(LaneHeaderName)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 构造带 baggage.mosi.lane=feat-foo 的 ctx
	m, _ := baggage.NewMember(BaggageLane, "feat-foo")
	bag, _ := baggage.New(m)
	ctx := baggage.ContextWithBaggage(context.Background(), bag)

	client := NewHTTPClient()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if capturedHeader != "feat-foo" {
		t.Errorf("expected X-Lane=feat-foo, got %q", capturedHeader)
	}
}

func TestLaneHeaderRoundTripper_NoLaneInBaggage_NoXLane(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get(LaneHeaderName)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient()
	// ctx 没带 baggage
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if capturedHeader != "" {
		t.Errorf("expected no X-Lane header, got %q", capturedHeader)
	}
}

func TestLaneHeaderRoundTripper_DoesNotOverrideExplicit(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get(LaneHeaderName)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m, _ := baggage.NewMember(BaggageLane, "feat-foo")
	bag, _ := baggage.New(m)
	ctx := baggage.ContextWithBaggage(context.Background(), bag)

	client := NewHTTPClient()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	req.Header.Set(LaneHeaderName, "explicit-override") // 业务显式 set
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if capturedHeader != "explicit-override" {
		t.Errorf("baggage 不应该覆盖业务显式 set，got %q", capturedHeader)
	}
}

func TestMergeLaneHeaderIntoBaggage(t *testing.T) {
	t.Run("X-Lane 写进空 baggage", func(t *testing.T) {
		ctx := mergeLaneHeaderIntoBaggage(context.Background(), "feat-foo")
		got := baggage.FromContext(ctx).Member(BaggageLane).Value()
		if got != "feat-foo" {
			t.Errorf("got %q want feat-foo", got)
		}
	})

	t.Run("空 X-Lane 不动 ctx", func(t *testing.T) {
		ctx := mergeLaneHeaderIntoBaggage(context.Background(), "")
		if v := baggage.FromContext(ctx).Member(BaggageLane).Value(); v != "" {
			t.Errorf("空 header 不应写 baggage，got %q", v)
		}
	})

	t.Run("baggage 已有 lane 不覆盖", func(t *testing.T) {
		m, _ := baggage.NewMember(BaggageLane, "from-baggage")
		bag, _ := baggage.New(m)
		ctx := baggage.ContextWithBaggage(context.Background(), bag)
		ctx = mergeLaneHeaderIntoBaggage(ctx, "from-header")
		got := baggage.FromContext(ctx).Member(BaggageLane).Value()
		if got != "from-baggage" {
			t.Errorf("baggage 应优先，got %q want from-baggage", got)
		}
	})
}
