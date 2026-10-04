// Package obs 提供统一的结构化 logger + OTel tracer + Gin 中间件。
//
// # 业务代码必须用 *Context 变体
//
// log 想自动带 trace_id / lane / tenant_id / user_id，**唯一办法是把
// ctx 喂给 logger**——Go 没有 thread-local，包没办法"自动魔法"拿到当前
// 请求的 ctx。Hertz hlog.CtxInfof / Go slog.InfoContext 都是这套设计。
//
// 写法：
//
//	// service / handler 业务代码（推荐 — 自带 trace 和 baggage 字段）
//	obs.WithModule("file-asset").InfoContext(ctx, "uploaded", "asset_id", id)
//	obs.InfoContext(ctx, "msg", "k", v)
//
//	// 等价写法
//	obs.WithModule("xxx").WithContext(ctx).Info("msg", "k", v)
//
// 不带 ctx 的 Info/Error 等方法**只用于 main 启动期 / 后台 daemon 没请求 ctx
// 的场景**——业务代码用 = 没 trace_id，Loki 里查不到，排障痛苦。
package obs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds all options for Init.
// 所有字段都有默认值，可以从环境变量自动读取：
//   - Service:      OTEL_SERVICE_NAME > NACOS_DATA_ID > "mosi-matrix"
//   - OTelEndpoint: OTelEndpoint > OTEL_EXPORTER_OTLP_ENDPOINT > "" (disabled)
//   - Level:        Level > LOG_LEVEL > "info"
//   - Format:       Format > LOG_FORMAT > "json"
//   - MaxLineBytes: MaxLineBytes > LOG_MAX_LINE_BYTES > 4096, minimum effective value 512
type Config struct {
	Level        string // debug, info, warn, error（默认从 LOG_LEVEL 读）
	Service      string // 服务名（默认从 OTEL_SERVICE_NAME 或 NACOS_DATA_ID 读）
	Format       string // "json" (default) or "text"（默认从 LOG_FORMAT 读）
	OTelEndpoint string // OTel Collector gRPC endpoint（默认从 OTEL_EXPORTER_OTLP_ENDPOINT 读）
	MaxLineBytes int    // 单行日志最大字节数，默认 4096；最低有效值 512
}

// Logger provides a key-value style API over a configured or deferred logrus entry.
type Logger struct {
	entry          *logrus.Entry
	deferredFields logrus.Fields
	followDefault  bool
}

var (
	rootLog        *logrus.Logger
	defaultLogger  atomic.Pointer[Logger]
	tracerShutdown func(context.Context) error
	initOnce       sync.Once
)

const (
	defaultService         = "mosi-matrix"
	defaultLogMaxLineBytes = 4096
	minLogMaxLineBytes     = 512
)

// Init initialises the global logger and optionally the OTel tracer provider.
// Safe to call once; subsequent calls are no-ops.
func Init(cfg Config) error {
	var initErr error
	initOnce.Do(func() {
		// 自动从环境变量读取默认值
		service := cfg.Service
		if service == "" {
			service = os.Getenv("OTEL_SERVICE_NAME")
		}
		if service == "" {
			service = os.Getenv("NACOS_DATA_ID")
		}
		if service == "" {
			service = defaultService
		}
		if cfg.Level == "" {
			cfg.Level = envOr("LOG_LEVEL", "info")
		}
		if cfg.Format == "" {
			cfg.Format = envOr("LOG_FORMAT", "json")
		}
		if cfg.MaxLineBytes <= 0 {
			cfg.MaxLineBytes = envIntOr("LOG_MAX_LINE_BYTES", defaultLogMaxLineBytes)
		}
		cfg.MaxLineBytes = normalizeLogMaxLineBytes(cfg.MaxLineBytes)

		// Logger
		rootLog = logrus.New()
		rootLog.SetOutput(os.Stdout)
		rootLog.SetReportCaller(false)

		var formatter logrus.Formatter
		switch cfg.Format {
		case "text":
			formatter = &logrus.TextFormatter{
				FullTimestamp:   true,
				TimestampFormat: "2006-01-02 15:04:05.000",
			}
		default:
			formatter = &logrus.JSONFormatter{
				TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			}
		}
		rootLog.SetFormatter(boundedFormatter{
			inner:        formatter,
			maxLineBytes: cfg.MaxLineBytes,
		})

		lvl, err := logrus.ParseLevel(cfg.Level)
		if err != nil {
			lvl = logrus.InfoLevel
		}
		rootLog.SetLevel(lvl)
		logger := &Logger{entry: rootLog.WithField("service", service)}
		lazyMu.Lock()
		defaultLogger.Store(logger)
		lazyMu.Unlock()

		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))

		// OTel Tracer (optional)
		endpoint := cfg.OTelEndpoint
		if endpoint == "" {
			endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if endpoint != "" {
			shutdown, err := InitTracer(service, endpoint)
			if err != nil {
				logger.Warn("failed to init otel tracer", "error", err)
			} else {
				tracerShutdown = shutdown
				logger.Info("otel tracer initialized", "endpoint", endpoint)
			}
		}
	})
	return initErr
}

// Close flushes the OTel tracer provider. Call from main before exit.
func Close() {
	if tracerShutdown != nil {
		_ = tracerShutdown(context.Background())
		tracerShutdown = nil
	}
}

// InitTracer initializes the OpenTelemetry tracer provider.
func InitTracer(service, endpoint string) (func(context.Context) error, error) {
	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
		)),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// Default returns the global logger.
//
// 正常路径：业务在 main 调 obs.Init(...) 后 defaultLogger 已就位。
// 兜底路径：用户没调 Init 直接 logging（库代码、main 漏调）→ 走 fallback
// logger 让程序不崩。但 fallback 里 propagator 是 OTel no-op，**trace_id 永远不会
// 出现在日志里、span 永远不会上报 Tempo**——典型 silent failure。
//
// 为了让漏调 Init 的服务暴露出来，fallback 第一次被取的时候往 stderr 红字打一
// 行，CI 日志/k8s logs 一眼能看到。生产 main.go 应该都调过 Init。
func Default() *Logger {
	if logger := defaultLogger.Load(); logger != nil {
		return logger
	}
	lazyMu.Lock()
	defer lazyMu.Unlock()
	if logger := defaultLogger.Load(); logger != nil {
		return logger
	}
	if fallbackLogger == nil {
		fb := logrus.New()
		fb.SetOutput(os.Stdout)
		fb.SetLevel(logrus.InfoLevel)
		fb.SetFormatter(boundedFormatter{
			inner: &logrus.JSONFormatter{
				TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			},
			maxLineBytes: normalizeLogMaxLineBytes(envIntOr("LOG_MAX_LINE_BYTES", defaultLogMaxLineBytes)),
		})
		fallbackLogger = &Logger{
			entry:         fb.WithField("service", defaultService),
			followDefault: true,
		}
		fmt.Fprintln(os.Stderr, "\033[31m[obs] WARNING: obs.Init() was never called — using fallback logger; trace_id and OTel spans will be MISSING from this service. Call obs.Init(obs.Config{}) at the top of main().\033[0m")
	}
	return fallbackLogger
}

var (
	lazyMu         sync.Mutex
	fallbackLogger *Logger
)

// WithModule returns a derived Logger with a "module" field.
func WithModule(module string) *Logger {
	fields := logrus.Fields{"module": module}
	if logger := defaultLogger.Load(); logger != nil {
		return logger.withFields(fields)
	}
	return newFollowingLogger(fields)
}

func (l *Logger) WithModule(module string) *Logger {
	return l.withFields(logrus.Fields{"module": module})
}

// WithContext returns a derived Logger with trace_id and span_id from OTel context.
func WithContext(ctx context.Context) *Logger {
	fields := traceFields(ctx)
	if logger := defaultLogger.Load(); logger != nil {
		return logger.withFields(fields)
	}
	return newFollowingLogger(fields)
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := traceFields(ctx)
	if len(fields) == 0 {
		return l
	}
	return l.withFields(fields)
}

// With returns a derived Logger with additional key-value pairs.
func (l *Logger) With(keysAndValues ...any) *Logger {
	return l.withFields(kvToFields(keysAndValues))
}

// Context-aware logging methods.
func (l *Logger) DebugContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.WithContext(ctx).Debug(msg, keysAndValues...)
}
func (l *Logger) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.WithContext(ctx).Info(msg, keysAndValues...)
}
func (l *Logger) WarnContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.WithContext(ctx).Warn(msg, keysAndValues...)
}
func (l *Logger) ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.WithContext(ctx).Error(msg, keysAndValues...)
}

// Standard logging methods.
func (l *Logger) Debug(msg string, keysAndValues ...any) {
	l.resolveEntry().WithFields(kvToFields(keysAndValues)).Debug(msg)
}
func (l *Logger) Info(msg string, keysAndValues ...any) {
	l.resolveEntry().WithFields(kvToFields(keysAndValues)).Info(msg)
}
func (l *Logger) Warn(msg string, keysAndValues ...any) {
	l.resolveEntry().WithFields(kvToFields(keysAndValues)).Warn(msg)
}
func (l *Logger) Error(msg string, keysAndValues ...any) {
	l.resolveEntry().WithFields(kvToFields(keysAndValues)).Error(msg)
}

// newFollowingLogger returns a Logger that resolves the process logger at write
// time. This keeps package-level loggers created during Go package init from
// retaining the fallback logger after the application calls Init.
func newFollowingLogger(fields logrus.Fields) *Logger {
	return &Logger{
		deferredFields: cloneFields(fields),
		followDefault:  true,
	}
}

func (l *Logger) withFields(fields logrus.Fields) *Logger {
	if len(fields) == 0 {
		return l
	}
	if l == nil || (l.entry == nil && !l.followDefault) {
		return newFollowingLogger(fields)
	}
	if !l.followDefault {
		return &Logger{entry: l.entry.WithFields(fields)}
	}
	return &Logger{
		entry:          l.entry,
		deferredFields: mergeFields(l.deferredFields, fields),
		followDefault:  true,
	}
}

func (l *Logger) resolveEntry() *logrus.Entry {
	if l == nil {
		return Default().resolveEntry()
	}
	if !l.followDefault && l.entry != nil {
		return l.entry
	}

	entry := l.entry
	if logger := defaultLogger.Load(); logger != nil {
		entry = logger.entry
	}
	if entry == nil {
		entry = Default().entry
	}
	return entry.WithFields(l.deferredFields)
}

func cloneFields(fields logrus.Fields) logrus.Fields {
	return mergeFields(nil, fields)
}

func mergeFields(base, extra logrus.Fields) logrus.Fields {
	fields := make(logrus.Fields, len(base)+len(extra))
	for key, value := range base {
		fields[key] = value
	}
	for key, value := range extra {
		fields[key] = value
	}
	return fields
}

// traceFields 从 ctx 抽 OTel trace + baggage 字段写日志。
//
// trace 字段（trace_id/span_id/trace_flags）让日志能跟 Tempo 串起来，
// baggage 字段（lane/tenant_id/user_id）让按业务维度过滤日志直接走 Loki，
// 不用先去 Tempo 翻 baggage（实测排障最常用就是按 lane 过滤）。
func traceFields(ctx context.Context) logrus.Fields {
	if ctx == nil {
		return nil
	}
	fields := logrus.Fields{}
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		fields["trace_id"] = sc.TraceID().String()
		fields["span_id"] = sc.SpanID().String()
		fields["trace_flags"] = sc.TraceFlags().String()
	}
	bag := baggage.FromContext(ctx)
	if v := bag.Member(BaggageLane).Value(); v != "" {
		fields["lane"] = v
	}
	if v := bag.Member(BaggageTenantID).Value(); v != "" {
		fields["tenant_id"] = v
	}
	if v := bag.Member(BaggageUserID).Value(); v != "" {
		fields["user_id"] = v
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func kvToFields(keysAndValues []any) logrus.Fields {
	if len(keysAndValues) == 0 {
		return nil
	}
	f := make(logrus.Fields)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		k, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		f[k] = keysAndValues[i+1]
	}
	return f
}

// Package-level convenience wrappers.
func Debug(msg string, keysAndValues ...any) { Default().Debug(msg, keysAndValues...) }
func Info(msg string, keysAndValues ...any)  { Default().Info(msg, keysAndValues...) }
func Warn(msg string, keysAndValues ...any)  { Default().Warn(msg, keysAndValues...) }
func Error(msg string, keysAndValues ...any) { Default().Error(msg, keysAndValues...) }

func DebugContext(ctx context.Context, msg string, keysAndValues ...any) {
	Default().DebugContext(ctx, msg, keysAndValues...)
}
func InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
	Default().InfoContext(ctx, msg, keysAndValues...)
}
func WarnContext(ctx context.Context, msg string, keysAndValues ...any) {
	Default().WarnContext(ctx, msg, keysAndValues...)
}
func ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {
	Default().ErrorContext(ctx, msg, keysAndValues...)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
