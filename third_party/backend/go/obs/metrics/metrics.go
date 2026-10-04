// Package metrics 提供每接口 RED 指标的 Gin 中间件。
//
// 独立于 obs 主包的子模块（同 gormlogger 模式）：只有接入指标的服务
// 才引入 prometheus 依赖，不影响其他 obs 使用方。
//
// 接入契约：
//   - 全局只 Use 一次；Handler 挂在根级 /metrics，且与 Deployment 的
//     prometheus.io/path 注解一致（中间件按 route=="/metrics" 排除自抓取）。
//   - SSE/流式接口的 duration = 整条流的总时长（流结束才计数），不是首字节时间。
//   - gin RedirectTrailingSlash 触发的 301 不进中间件链，是已知计量盲区。
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// httpRequestsTotal 与 httpRequestDuration 的 _count 逐 series 恒等，
	// 保留 counter 只为惯例命名好用；看板/告警两者用其一即可。
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of HTTP requests handled, by route template.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_server_request_duration_seconds",
		Help: "Duration of HTTP requests in seconds, by route template.",
		// 默认桶只到 10s，长推理/流式接口（上限 7200s）的分位数会在 +Inf 饱和，
		// histogram_quantile 会给出"假确定值"，必须补长尾桶。
		Buckets: append(prometheus.DefBuckets, 30, 60, 120, 300, 600, 1800, 3600, 7200),
	}, []string{"method", "route", "status"})
)

// GinMiddleware 每接口 RED 指标中间件。
//
// route 取 gin 注册路由模板（c.FullPath()，如 /v1/users/:id），天然有界；
// 未匹配任何路由的请求（扫描器/探测）统一折叠进 route="unmatched"，
// method 白名单归一（未知方法折叠 "OTHER"）——原始 path 和自定义 method
// 都是请求方可控的，禁止进 label（高基数防线）。
//
// 记账在 defer 中完成：panic 的请求按 500 计入后原样重抛，不依赖与
// Recovery 的挂载顺序（gin.Default() 的 Recovery 在外层也能正确计数）。
//
// 指标命名对齐 OTel HTTP Semantic Conventions 的 Prometheus 形态：
//
//	http_server_requests_total{method,route,status}
//	http_server_request_duration_seconds{method,route,status}
//
// 用法：
//
//	engine.Use(metrics.GinMiddleware())
//	engine.GET("/metrics", gin.WrapH(metrics.Handler()))
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		defer func() {
			rec := recover()
			if !isHealthProbe(c.Request) {
				route := c.FullPath()
				if route != "/metrics" {
					if route == "" {
						route = "unmatched"
					}
					status := c.Writer.Status()
					if rec != nil {
						status = http.StatusInternalServerError
					}
					s := strconv.Itoa(status)
					m := normalizeMethod(c.Request.Method)
					httpRequestsTotal.WithLabelValues(m, route, s).Inc()
					httpRequestDuration.WithLabelValues(m, route, s).Observe(time.Since(start).Seconds())
				}
			}
			if rec != nil {
				panic(rec)
			}
		}()

		c.Next()
	}
}

// Handler 返回 Prometheus 指标的 HTTP handler（默认 registry，
// 自带 go_*/process_* 运行时指标，与 inference-orchestrator io_* 同形态）。
// 注意：默认 registry 重名注册会在启动期 panic（fail-fast），接入前确认
// 服务没有自己注册过同名 http_server_* 指标。
func Handler() http.Handler {
	return promhttp.Handler()
}

// normalizeMethod 把 method 折叠进白名单，未知方法归 "OTHER"（OTel _OTHER 语义）。
func normalizeMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
		return m
	}
	return "OTHER"
}

// isHealthProbe 与 obs.GinAccessLog 的探针过滤同口径。
func isHealthProbe(r *http.Request) bool {
	ua := r.UserAgent()
	return strings.HasPrefix(ua, "kube-probe/") || strings.HasPrefix(ua, "Envoy/")
}
