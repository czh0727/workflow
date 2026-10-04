// Package obs / baggage：W3C Baggage 业务字段命名规范常量。
//
// 命名规则（对齐 OTel attribute convention）：
//   - 全小写 + 点分隔
//   - matrix.* 前缀（产品命名空间，避免跟其它系统冲突）
//
// 使用：
//
//	import "go.opentelemetry.io/otel/baggage"
//
//	// 写
//	m, _ := baggage.NewMember(obs.BaggageLane, "feat-foo")
//	bag, _ := baggage.New(m)
//	ctx = baggage.ContextWithBaggage(ctx, bag)
//
//	// 读
//	lane := baggage.FromContext(ctx).Member(obs.BaggageLane).Value()

package obs

const (
	// BaggageLane 当前请求的 lane 标识（baseline / feat-xxx / hotfix-xxx）。
	// 用于流量染色：浏览器/网关入口塞，跨服务跳保持，支撑 lane 隔离路由。
	BaggageLane = "matrix.lane"

	// BaggageTenantID 多租户场景下当前调用归属租户 ID。
	BaggageTenantID = "matrix.tenant.id"

	// BaggageUserID 当前操作用户 canonical ID（"liguoxin"）。
	// 跟 OTel logging 的 trace_id 配合，支持"按用户查链路"。
	BaggageUserID = "matrix.user.id"

	// BaggageRequestSource 流量来源类别：browser / cron / replay / migrate-script。
	// 区分人类操作 vs 自动任务，用于审计和限流策略。
	BaggageRequestSource = "matrix.request.source"
)

// LaneHeaderName 为兼容现有 Istio HTTPRoute 匹配，outbound 请求同时
// 写一份 lane 值到这个 plain header。终态会迁到只看 baggage。
const LaneHeaderName = "x-lane"
