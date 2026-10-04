package obs

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func resetObsForTest(t *testing.T) {
	t.Helper()
	rootLog = nil
	defaultLogger.Store(nil)
	fallbackLogger = nil
	tracerShutdown = nil
	initOnce = sync.Once{}
	t.Cleanup(func() {
		rootLog = nil
		defaultLogger.Store(nil)
		fallbackLogger = nil
		tracerShutdown = nil
		initOnce = sync.Once{}
	})
}

func initObsForTest(t *testing.T, cfg Config, out *bytes.Buffer) {
	t.Helper()
	resetObsForTest(t)
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if rootLog == nil {
		t.Fatal("rootLog is nil")
	}
	rootLog.SetOutput(out)
}

func lastLogLine(t *testing.T, out *bytes.Buffer) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[len(lines)-1] == "" {
		t.Fatal("no log output")
	}
	return lines[len(lines)-1]
}

func TestJSONLoggerBoundsOversizedLine(t *testing.T) {
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc", MaxLineBytes: 512}, &out)

	WithModule("runtime").Info("payload.received",
		"body", strings.Repeat("x", 10_000),
		"trace_id", "67aa5c950854e17d68072983b7af9296",
		"tenant_id", "tenant-a",
	)

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 512 {
		t.Fatalf("log line bytes=%d, want <= 512", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if decoded["msg"] != "payload.received" {
		t.Fatalf("msg=%v, want payload.received", decoded["msg"])
	}
	if decoded["service"] != "test-svc" {
		t.Fatalf("service=%v, want test-svc", decoded["service"])
	}
	if decoded["module"] != "runtime" {
		t.Fatalf("module=%v, want runtime", decoded["module"])
	}
	if decoded["trace_id"] != "67aa5c950854e17d68072983b7af9296" {
		t.Fatalf("trace_id=%v", decoded["trace_id"])
	}
	if decoded["tenant_id"] != "tenant-a" {
		t.Fatalf("tenant_id=%v", decoded["tenant_id"])
	}
	if decoded["log_truncated"] != true {
		t.Fatalf("log_truncated=%v, want true", decoded["log_truncated"])
	}
	if decoded["log_max_bytes"] != float64(512) {
		t.Fatalf("log_max_bytes=%v, want 512", decoded["log_max_bytes"])
	}
	if original, ok := decoded["log_original_bytes"].(float64); !ok || original <= 512 {
		t.Fatalf("log_original_bytes=%v, want > 512", decoded["log_original_bytes"])
	}
	if decoded["body"] == strings.Repeat("x", 10_000) {
		t.Fatal("body was not truncated")
	}
}

func TestJSONLoggerHonorsEnvMaxLineBytes(t *testing.T) {
	t.Setenv("LOG_MAX_LINE_BYTES", "512")
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc"}, &out)

	Info("oversized", "payload", strings.Repeat("y", 10_000))

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 512 {
		t.Fatalf("log line bytes=%d, want <= 512", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if decoded["log_max_bytes"] != float64(512) {
		t.Fatalf("log_max_bytes=%v, want 512", decoded["log_max_bytes"])
	}
}

func TestJSONLoggerHonorsSDKMaxLineBytesOverEnv(t *testing.T) {
	t.Setenv("LOG_MAX_LINE_BYTES", "512")
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc", MaxLineBytes: 1024}, &out)

	Info("oversized", "payload", strings.Repeat("y", 10_000))

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 1024 {
		t.Fatalf("log line bytes=%d, want <= 1024", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if decoded["log_max_bytes"] != float64(1024) {
		t.Fatalf("log_max_bytes=%v, want 1024", decoded["log_max_bytes"])
	}
}

func TestJSONLoggerInvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("LOG_MAX_LINE_BYTES", "invalid")
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc"}, &out)

	Info("oversized", "payload", strings.Repeat("z", 10_000))

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 4096 {
		t.Fatalf("log line bytes=%d, want <= 4096", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if decoded["log_max_bytes"] != float64(4096) {
		t.Fatalf("log_max_bytes=%v, want 4096", decoded["log_max_bytes"])
	}
}

func TestJSONLoggerClampsTooSmallMaxLineBytes(t *testing.T) {
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc", MaxLineBytes: 64}, &out)

	Info("oversized", "payload", strings.Repeat("z", 10_000))

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 512 {
		t.Fatalf("log line bytes=%d, want <= 512", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if decoded["log_max_bytes"] != float64(512) {
		t.Fatalf("log_max_bytes=%v, want 512", decoded["log_max_bytes"])
	}
}

func TestJSONLoggerClampsTooSmallEnvMaxLineBytes(t *testing.T) {
	t.Setenv("LOG_MAX_LINE_BYTES", "64")
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc"}, &out)

	Info("oversized", "payload", strings.Repeat("z", 10_000))

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 512 {
		t.Fatalf("log line bytes=%d, want <= 512", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if decoded["log_max_bytes"] != float64(512) {
		t.Fatalf("log_max_bytes=%v, want 512", decoded["log_max_bytes"])
	}
}

func TestJSONLoggerTruncatesNestedPayloadFields(t *testing.T) {
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc", MaxLineBytes: 512}, &out)

	Info("nested.payload", "payload", map[string]any{
		"body": strings.Repeat("x", 10_000),
		"keep": "ok",
	})

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 512 {
		t.Fatalf("log line bytes=%d, want <= 512", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	payload, ok := decoded["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload=%T, want object", decoded["payload"])
	}
	if payload["keep"] != "ok" {
		t.Fatalf("payload.keep=%v, want ok", payload["keep"])
	}
	if payload["body"] == strings.Repeat("x", 10_000) {
		t.Fatal("nested payload body was not truncated")
	}
}

func TestTextLoggerBoundsOversizedLine(t *testing.T) {
	var out bytes.Buffer
	initObsForTest(t, Config{Service: "test-svc", Format: "text", MaxLineBytes: 512}, &out)

	WithModule("runtime").Info("payload.received", "body", strings.Repeat("x", 10_000))

	line := lastLogLine(t, &out)
	if got := len([]byte(line)); got > 512 {
		t.Fatalf("log line bytes=%d, want <= 512", got)
	}
	for _, want := range []string{"payload.received", "service=test-svc", "module=runtime", "log_truncated=true", "log_max_bytes=512"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line missing %q: %s", want, line)
		}
	}
}
