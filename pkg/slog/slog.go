// Package slog 配置 Workflow 服务日志。
package slog

import (
	"context"
	stdslog "log/slog"
	"os"
	"strconv"
	"strings"

	sharedobs "git.sotatts.online/matrix/matrix/packages/backend/go/obs"
	"github.com/go-kratos/kratos/v3/log"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
)

// Init 创建 Kratos 日志器，并将其设置为进程默认日志器。
func Init(serviceID, serviceName, serviceVersion string) *stdslog.Logger {
	level := stdslog.LevelInfo
	switch value := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))); value {
	case "debug":
		level = stdslog.LevelDebug
	case "warn", "warning":
		level = stdslog.LevelWarn
	case "error":
		level = stdslog.LevelError
	}
	maxLineBytes := defaultLogMaxLineBytes
	if value := strings.TrimSpace(os.Getenv("LOG_MAX_LINE_BYTES")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			maxLineBytes = parsed
		}
	}
	if maxLineBytes < minLogMaxLineBytes {
		maxLineBytes = minLogMaxLineBytes
	}

	handler := stdslog.NewJSONHandler(
		boundedWriter{out: os.Stdout, maxLineBytes: maxLineBytes},
		&stdslog.HandlerOptions{
			AddSource: true,
			Level:     level,
			ReplaceAttr: func(_ []string, attr stdslog.Attr) stdslog.Attr {
				if attr.Key == stdslog.LevelKey {
					attr.Value = stdslog.StringValue(strings.ToLower(attr.Value.String()))
				}
				return attr
			},
		},
	)
	logger := log.NewLogger(
		handler,
		log.WithExtractor(func(ctx context.Context) []stdslog.Attr {
			attrs := make([]stdslog.Attr, 0, 6)
			if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
				attrs = append(attrs,
					stdslog.String("trace_id", spanContext.TraceID().String()),
					stdslog.String("span_id", spanContext.SpanID().String()),
					stdslog.String("trace_flags", spanContext.TraceFlags().String()),
				)
			}
			bag := baggage.FromContext(ctx)
			if value := bag.Member(sharedobs.BaggageLane).Value(); value != "" {
				attrs = append(attrs, stdslog.String("lane", value))
			}
			if value := bag.Member(sharedobs.BaggageTenantID).Value(); value != "" {
				attrs = append(attrs, stdslog.String("tenant_id", value))
			}
			if value := bag.Member(sharedobs.BaggageUserID).Value(); value != "" {
				attrs = append(attrs, stdslog.String("user_id", value))
			}
			return attrs
		}),
	).With(
		stdslog.String("service.id", serviceID),
		stdslog.String("service.name", serviceName),
		stdslog.String("service.version", serviceVersion),
	)
	log.SetDefault(logger)
	return logger
}
