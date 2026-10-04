// Package obs / middleware：Gin server 端 propagation 中间件。
//
// 装这个之后：
//  1. 从 inbound 请求 header extract W3C TraceContext (traceparent) + Baggage 到 ctx
//  2. 兼容前端/网关入口直接发 X-Lane header（不带 baggage 的请求），
//     把 X-Lane 值写进 baggage.mosi.lane，让下游 obs.NewHTTPClient
//     的 outbound 自动透传
//  3. tracer.Start 起 SERVER span，c.Request.WithContext(ctx) 注回
//
// 用法：
//
//	engine.Use(obs.GinPropagationMiddleware("studio-backend"))
//
// 通常作为最早的中间件之一（在 GinAccessLog 之前），让所有后续 handler
// 都拿到带 trace + baggage 的 ctx。

package obs

import (
	"context"
	"crypto/rand"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const xTraceIDHeaderName = "X-Trace-Id"

// GinPropagationMiddleware 全局注 trace + baggage extractor，并补 X-Lane → baggage 兼容。
// serviceName 用作 tracer name 和 span attribute。
func GinPropagationMiddleware(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)
	return func(c *gin.Context) {
		// 1. 从 header extract trace + baggage（OTel 全局 propagator）
		ctx := otel.GetTextMapPropagator().Extract(
			c.Request.Context(),
			propagation.HeaderCarrier(c.Request.Header),
		)
		if xTraceID := c.Request.Header.Get(xTraceIDHeaderName); xTraceID != "" {
			ctx = mergeXTraceIDIntoRemoteParent(ctx, xTraceID)
		} else {
			ctx = ensureRemoteParentSampled(ctx)
		}

		// 2. 兼容入口请求只发 X-Lane plain header 没 baggage 的情况：
		//    把 X-Lane 值合并进 baggage.mosi.lane，下游 outbound 自动透传
		ctx = mergeLaneHeaderIntoBaggage(ctx, c.Request.Header.Get(LaneHeaderName))

		// 3. 起 server span（保留 inbound trace 接续）
		spanName := c.Request.Method + " " + c.Request.URL.Path
		ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		if c.Request.Header.Get(xTraceIDHeaderName) == "" {
			traceID := span.SpanContext().TraceID().String()
			c.Request.Header.Set(xTraceIDHeaderName, traceID)
			c.Header(xTraceIDHeaderName, traceID)
		}
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		// 4. 标准属性 + 5xx 标记 error
		status := c.Writer.Status()
		span.SetAttributes(
			attribute.String("http.request.method", c.Request.Method),
			attribute.String("url.path", c.Request.URL.Path),
			attribute.String("http.route", c.FullPath()),
			attribute.Int("http.response.status_code", status),
		)
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	}
}

func mergeXTraceIDIntoRemoteParent(ctx context.Context, xTraceID string) context.Context {
	if xTraceID == "" {
		return ctx
	}
	tid, err := trace.TraceIDFromHex(xTraceID)
	if err != nil || !tid.IsValid() {
		return ctx
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() && sc.TraceID() == tid {
		return contextWithSampledRemoteSpanContext(ctx, sc)
	}
	var sid trace.SpanID
	if _, err := rand.Read(sid[:]); err != nil || !sid.IsValid() {
		sid = trace.SpanID{1}
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

func ensureRemoteParentSampled(ctx context.Context) context.Context {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ctx
	}
	return contextWithSampledRemoteSpanContext(ctx, sc)
}

func contextWithSampledRemoteSpanContext(ctx context.Context, sc trace.SpanContext) context.Context {
	if sc.TraceFlags().IsSampled() && sc.IsRemote() {
		return ctx
	}
	sampled := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    sc.TraceID(),
		SpanID:     sc.SpanID(),
		TraceFlags: sc.TraceFlags() | trace.FlagsSampled,
		TraceState: sc.TraceState(),
		Remote:     true,
	})
	return trace.ContextWithRemoteSpanContext(ctx, sampled)
}

// mergeLaneHeaderIntoBaggage 当 inbound 带了 X-Lane 但 baggage 里没对应 entry 时，
// 把 X-Lane 值合并进 baggage。已有 baggage.mosi.lane 不覆盖（baggage 优先）。
//
// 这是迁移期 compat layer：浏览器/前端短期还是发 X-Lane header（业务不改 fetch 代码），
// 由 mesh 入口第一跳服务把它"翻译"成 baggage，往后所有跳就走标准 baggage propagation。
func mergeLaneHeaderIntoBaggage(ctx context.Context, laneHeader string) context.Context {
	if laneHeader == "" {
		return ctx
	}
	bag := baggage.FromContext(ctx)
	if bag.Member(BaggageLane).Value() != "" {
		// baggage 已经有了，以 baggage 为准（上游服务可能已经写过）
		return ctx
	}
	m, err := baggage.NewMember(BaggageLane, laneHeader)
	if err != nil {
		return ctx // 非法 lane 值（比如有非法字符），静默跳过
	}
	newBag, err := bag.SetMember(m)
	if err != nil {
		return ctx
	}
	return baggage.ContextWithBaggage(ctx, newBag)
}
