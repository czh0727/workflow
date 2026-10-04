package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// scrapeMetrics 请求 Handler 并返回指标文本。
func scrapeMetrics(t *testing.T) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler().ServeHTTP(w, req)
	body, err := io.ReadAll(w.Result().Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	return string(body)
}

func TestGinMiddlewareRecordsRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware())
	r.GET("/v1/widgets/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/widgets/42", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}

	out := scrapeMetrics(t)
	want := `http_server_requests_total{method="GET",route="/v1/widgets/:id",status="200"}`
	if !strings.Contains(out, want) {
		t.Fatalf("metrics output missing %q", want)
	}
	wantDur := `http_server_request_duration_seconds_count{method="GET",route="/v1/widgets/:id",status="200"}`
	if !strings.Contains(out, wantDur) {
		t.Fatalf("metrics output missing %q", wantDur)
	}
}

func TestGinMiddlewareCollapsesUnmatchedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware())

	// 两个不同的未注册路径必须折叠进同一个 route="unmatched"，不能把原始 path 当 label（基数防线）
	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "unmatched", "404"))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/scan/probe/admin", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/scan/probe/env", nil))
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "unmatched", "404"))
	if after-before != 2 {
		t.Fatalf("unmatched requests not folded: delta=%v want 2", after-before)
	}

	out := scrapeMetrics(t)
	if strings.Contains(out, "/scan/probe") {
		t.Fatalf("raw unmatched path leaked into metrics labels")
	}
}

func TestGinMiddlewareSkipsHealthProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware())
	r.GET("/probe-only-route", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/probe-only-route", nil)
	req.Header.Set("User-Agent", "kube-probe/1.29")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if out := scrapeMetrics(t); strings.Contains(out, "/probe-only-route") {
		t.Fatalf("health probe request should not be recorded")
	}
}

func TestGinMiddlewareSkipsMetricsEndpointItself(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware())
	r.GET("/metrics", gin.WrapH(Handler()))

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", nil))

	out := scrapeMetrics(t)
	if strings.Contains(out, `route="/metrics"`) {
		t.Fatalf("scrape of /metrics should not count itself")
	}
}

func TestGinMiddlewareCountsPanicAs500EvenWhenRecoveryIsOuter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// gin.Default() 的真实顺序：Recovery 在外层。计量不允许依赖挂载顺序。
	r.Use(gin.Recovery(), GinMiddleware())
	r.GET("/panic-route", func(c *gin.Context) { panic("boom") })

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/panic-route", "500"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic-route", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/panic-route", "500"))
	if after-before != 1 {
		t.Fatalf("panic request not counted as 500: delta=%v", after-before)
	}
}

func TestGinMiddlewareNormalizesUnknownMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware())

	// method 是请求方可控的 token，必须白名单归一，否则是半无界 label
	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("OTHER", "unmatched", "404"))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("FOOBAR", "/method-norm-probe", nil))
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("OTHER", "unmatched", "404"))
	if after-before != 1 {
		t.Fatalf("unknown method not folded into OTHER: delta=%v", after-before)
	}
	if out := scrapeMetrics(t); strings.Contains(out, `method="FOOBAR"`) {
		t.Fatalf("raw custom method leaked into metrics labels")
	}
}

func TestGinMiddlewareHistogramCoversLongRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware())
	r.GET("/bucket-probe", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/bucket-probe", nil))

	// 长推理接口超时上限 7200s，默认桶只到 10s 会让分位数饱和失真
	if out := scrapeMetrics(t); !strings.Contains(out, `le="7200"`) {
		t.Fatalf("histogram buckets must cover long inference requests (le=7200 missing)")
	}
}
