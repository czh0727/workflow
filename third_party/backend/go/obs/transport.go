// Package obs / transport：lane-aware HTTP client。
//
// 业务用 obs.NewHTTPClient() 替代裸 &http.Client{} 后：
//  1. 自动 inject W3C TraceContext (traceparent) + Baggage 到 outbound
//     —— 借 otelhttp.NewTransport 实现，OTel 标准 propagator 全自动
//  2. 额外把 baggage 里的 mosi.lane 镜像到 X-Lane header
//     —— 兼容现有 Istio HTTPRoute 按 X-Lane exact match 路由的方式
//     —— 业务代码 0 改动就能让 lane 染色全链路保持
//
// 使用：
//
//	client := obs.NewHTTPClient()
//	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	resp, _ := client.Do(req)
//
// ctx 必须从 inbound request context 派生（GinPropagationMiddleware
// 已经把 baggage 写进 ctx），否则 RoundTripper 读不到 lane → 不写 X-Lane。

package obs

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/baggage"
)

// sharedTransport 是 http.DefaultTransport 的调优克隆，被所有 obs HTTP client 复用
// （共享同一个连接池）。
//
// 为什么克隆而不直接用 http.DefaultTransport：默认 MaxIdleConnsPerHost=2（Go 默认值，对
// 内部高 QPS 服务调用偏小）。突发并发调用会开一堆连接、用完只留 2 条空闲、其余立即被客户端
// 关闭 → 连接 churn 很高 → 偶发"请求复用到正被客户端关闭的连接"竞态 → `Post: EOF`。调大
// per-host 空闲池让连接保温复用，churn 暴降。⚠️ 必须 Clone：直接改全局 http.DefaultTransport
// 会污染整个进程里所有用默认 transport 的 HTTP 调用。
var sharedTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 200
	t.MaxIdleConnsPerHost = 100
	return t
}()

// NewHTTPClient 返回带 OTel propagator + lane header 镜像的 http.Client。
// Timeout 默认 30s，调用方可后续重设。
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: otelhttp.NewTransport(
			&laneHeaderRoundTripper{base: sharedTransport},
		),
	}
}

// NewHTTPClientWithBase 让调用方自己传底层 RoundTripper（比如已有 proxy 配置的 Transport）。
// 包装链：otelhttp(propagator inject) → laneHeader(写 X-Lane) → base
func NewHTTPClientWithBase(base http.RoundTripper) *http.Client {
	if base == nil {
		base = sharedTransport
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: otelhttp.NewTransport(
			&laneHeaderRoundTripper{base: base},
		),
	}
}

// laneHeaderRoundTripper 在请求出去之前，从 ctx 里读 baggage.mosi.lane
// 写到 X-Lane header。给 Istio HTTPRoute 按 X-Lane match 的现有规则用。
type laneHeaderRoundTripper struct {
	base http.RoundTripper
}

func (t *laneHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if v := baggage.FromContext(req.Context()).Member(BaggageLane).Value(); v != "" {
		// 不覆盖业务已显式 set 的 X-Lane（极少数手动场景）
		if req.Header.Get(LaneHeaderName) == "" {
			req.Header.Set(LaneHeaderName, v)
		}
	}
	return t.base.RoundTrip(req)
}
