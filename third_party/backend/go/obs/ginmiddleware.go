package obs

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func itoa(n int) string { return strconv.Itoa(n) }

// GinAccessLog 统一的 Gin 访问日志中间件。
// 字段遵循 OpenTelemetry HTTP Semantic Conventions。
//
// 特性：
//   - 结构化 JSON 日志（msg = "METHOD PATH STATUS"）
//   - 状态码决定日志级别（5xx=error / 4xx=warn / 其他=info）
//   - 5xx 自动把 OTel span 标记为 error（Tempo 里能看到失败调用）
//   - 自动附加 user_id / tenant_id（从 Gin Keys 里读，由鉴权中间件设置）
//   - kube-probe / Envoy 健康检查不记录
//   - trace_id / span_id 自动注入
//
// 用法：engine.Use(obs.GinAccessLog())
func GinAccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if rq := c.Request.URL.RawQuery; rq != "" {
			path = path + "?" + rq
		}

		c.Next()

		if isHealthProbe(c.Request) {
			return
		}

		status := c.Writer.Status()
		latencyMs := time.Since(start).Milliseconds()
		msg := c.Request.Method + " " + c.Request.URL.Path + " " + itoa(status)

		// 5xx 给 OTel span 打失败标记
		if status >= 500 {
			if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
		}

		fields := []any{
			"method", c.Request.Method,
			"path", path,
			"route", c.FullPath(),
			"status", status,
			"latency_ms", latencyMs,
			"client_ip", clientIP(c.Request),
			"user_agent", c.Request.UserAgent(),
			"referer", c.Request.Referer(),
			"request_id", requestID(c),
			"protocol", c.Request.Proto,
			"bytes_received", c.Request.ContentLength,
			"bytes_sent", c.Writer.Size(),
		}

		// 业务上下文（鉴权中间件把 user_id/tenant_id 写入 Gin Keys 即可自动采集）
		if v, ok := c.Get("user_id"); ok {
			fields = append(fields, "user_id", v)
		}
		if v, ok := c.Get("tenant_id"); ok {
			fields = append(fields, "tenant_id", v)
		}

		log := WithModule("access").WithContext(c.Request.Context())
		switch {
		case status >= 500:
			log.Error(msg, fields...)
		case status >= 400:
			log.Warn(msg, fields...)
		default:
			log.Info(msg, fields...)
		}
	}
}

func isHealthProbe(r *http.Request) bool {
	ua := r.UserAgent()
	return strings.HasPrefix(ua, "kube-probe/") || strings.HasPrefix(ua, "Envoy/")
}

func requestID(c *gin.Context) string {
	if v := c.GetHeader("X-Request-ID"); v != "" {
		return v
	}
	if v := c.GetHeader("X-Request-Id"); v != "" {
		return v
	}
	return ""
}

// clientIP 从 HTTP 请求中提取真实客户端 IP。
// 优先级：X-Envoy-External-Address > X-Forwarded-For > X-Real-IP > RemoteAddr
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Envoy-External-Address"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if idx := strings.Index(v, ","); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
