package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
)

func TestLoggerDerivedBeforeInitUsesInitializedLogger(t *testing.T) {
	resetObsForTest(t)

	traceID, err := trace.TraceIDFromHex("67aa5c950854e17d68072983b7af9296")
	if err != nil {
		t.Fatalf("parse trace ID: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("8e02e658703eb824")
	if err != nil {
		t.Fatalf("parse span ID: %v", err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
	lane, err := baggage.NewMember(BaggageLane, "feat-obs-lazy")
	if err != nil {
		t.Fatalf("create lane baggage: %v", err)
	}
	bag, err := baggage.New(lane)
	if err != nil {
		t.Fatalf("create baggage: %v", err)
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)

	log := WithModule("pre-init-module").
		With("component", "pre-init-component").
		WithContext(ctx)
	if fallbackLogger != nil {
		t.Fatal("deriving a logger before Init created the fallback logger")
	}

	var out bytes.Buffer
	if err := Init(Config{Service: "initialized-service"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	rootLog.SetOutput(&out)
	log.Info("post-init-log")

	assertLogFields(t, &out, map[string]any{
		"service":   "initialized-service",
		"module":    "pre-init-module",
		"component": "pre-init-component",
		"trace_id":  traceID.String(),
		"span_id":   spanID.String(),
		"lane":      "feat-obs-lazy",
	})
}

func TestLoggerDerivedBeforeInitWarnsOnFirstLogAndFollowsInit(t *testing.T) {
	resetObsForTest(t)

	log := WithModule("pre-init-module")
	if fallbackLogger != nil {
		t.Fatal("deriving a logger before Init created the fallback logger")
	}

	warning := captureStderr(t, func() {
		log.Info("first-pre-init-log")
		log.Info("second-pre-init-log")
	})
	if count := strings.Count(warning, "WARNING: obs.Init() was never called"); count != 1 {
		t.Fatalf("pre-Init warning count=%d, want 1: %q", count, warning)
	}

	var out bytes.Buffer
	if err := Init(Config{Service: "initialized-after-fallback"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	rootLog.SetOutput(&out)
	log.Info("post-init-log")
	assertLogFields(t, &out, map[string]any{
		"service": "initialized-after-fallback",
		"module":  "pre-init-module",
	})
}

func TestLoggerDerivedFromFallbackBeforeInitFollowsInit(t *testing.T) {
	resetObsForTest(t)

	var log *Logger
	warning := captureStderr(t, func() {
		log = Default().WithModule("fallback-derived").With("component", "fallback-component")
	})
	if count := strings.Count(warning, "WARNING: obs.Init() was never called"); count != 1 {
		t.Fatalf("pre-Init warning count=%d, want 1: %q", count, warning)
	}

	var out bytes.Buffer
	if err := Init(Config{Service: "initialized-after-default"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	rootLog.SetOutput(&out)
	log.Info("post-init-log")
	assertLogFields(t, &out, map[string]any{
		"service":   "initialized-after-default",
		"module":    "fallback-derived",
		"component": "fallback-component",
	})
}

func TestLoggerDerivedBeforeInitCanLogConcurrentWithInit(t *testing.T) {
	resetObsForTest(t)

	log := WithModule("concurrent-init")
	captureStderr(t, func() {
		Default().entry.Logger.SetOutput(io.Discard)
	})

	start := make(chan struct{})
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errCh <- Init(Config{Service: "concurrent-service", Level: "error"})
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1_000; i++ {
			log.Info("concurrent-log", "index", i)
		}
	}()
	close(start)
	wg.Wait()
	if err := <-errCh; err != nil {
		t.Fatalf("Init: %v", err)
	}

	var out bytes.Buffer
	rootLog.SetOutput(&out)
	log.Error("post-init-log")
	assertLogFields(t, &out, map[string]any{
		"service": "concurrent-service",
		"module":  "concurrent-init",
	})
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writeStderr
	fn()
	os.Stderr = oldStderr
	if err := writeStderr.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	warning, err := io.ReadAll(readStderr)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := readStderr.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return string(warning)
}

func assertLogFields(t *testing.T, out *bytes.Buffer, wants map[string]any) {
	t.Helper()
	line := lastLogLine(t, out)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	for key, want := range wants {
		if got := decoded[key]; got != want {
			t.Errorf("%s=%v, want %v", key, got, want)
		}
	}
}
