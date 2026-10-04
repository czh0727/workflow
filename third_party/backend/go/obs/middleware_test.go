package obs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestGinPropagationMiddlewareUsesXTraceIDWhenTraceparentMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	const xTraceID = "67aa5c950854e17d68072983b7af9296"
	var gotTraceID string

	r := gin.New()
	r.Use(GinPropagationMiddleware("test-service"))
	r.GET("/probe", func(c *gin.Context) {
		gotTraceID = trace.SpanFromContext(c.Request.Context()).SpanContext().TraceID().String()
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("X-Trace-Id", xTraceID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if gotTraceID != xTraceID {
		t.Fatalf("trace_id=%q want %q", gotTraceID, xTraceID)
	}
}

func TestGinPropagationMiddlewareUsesXTraceIDWhenTraceparentConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	const xTraceID = "600a89cc14678daec3c2cd43af34dd82"
	var gotTraceID string

	r := gin.New()
	r.Use(GinPropagationMiddleware("test-service"))
	r.GET("/probe", func(c *gin.Context) {
		gotTraceID = trace.SpanFromContext(c.Request.Context()).SpanContext().TraceID().String()
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("X-Trace-Id", xTraceID)
	req.Header.Set("traceparent", "00-9d2b39397a96d3bd9c5aaa4b8338da44-306dea44e2010de3-00")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if gotTraceID != xTraceID {
		t.Fatalf("trace_id=%q want %q", gotTraceID, xTraceID)
	}
}

func TestGinPropagationMiddlewarePublishesTraceIDWhenXTraceIDMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	const traceparentTraceID = "dbfed134fa36d6f227d13c4c333e26c8"
	var gotSpanTraceID string
	var gotSampled bool
	var gotHeaderTraceID string

	r := gin.New()
	r.Use(GinPropagationMiddleware("test-service"))
	r.GET("/probe", func(c *gin.Context) {
		sc := trace.SpanFromContext(c.Request.Context()).SpanContext()
		gotSpanTraceID = sc.TraceID().String()
		gotSampled = sc.TraceFlags().IsSampled()
		gotHeaderTraceID = c.GetHeader("X-Trace-Id")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("traceparent", "00-"+traceparentTraceID+"-8e02e658703eb824-00")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if gotSpanTraceID != traceparentTraceID {
		t.Fatalf("span trace_id=%q want %q", gotSpanTraceID, traceparentTraceID)
	}
	if !gotSampled {
		t.Fatal("span is not sampled")
	}
	if gotHeaderTraceID != gotSpanTraceID {
		t.Fatalf("header trace_id=%q want span trace_id %q", gotHeaderTraceID, gotSpanTraceID)
	}
	if w.Header().Get("X-Trace-Id") != gotSpanTraceID {
		t.Fatalf("response trace_id=%q want span trace_id %q", w.Header().Get("X-Trace-Id"), gotSpanTraceID)
	}
}
