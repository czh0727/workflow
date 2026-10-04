package request

import "context"

const (
	AppIDHeader     = "X-App-ID"
	OriginIDHeader  = "X-Origin-ID"
	SubjectIDHeader = "X-Subject-ID"
	RequestIDHeader = "X-Request-ID"
)

// Info 包含下游操作需要的请求级信息。
type Info struct {
	AppID     string
	OriginID  string
	SubjectID string
	RequestID string
}

type contextKey struct{}

// WithInfo 将请求信息保存到上下文中。
func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, contextKey{}, info)
}

// FromContext 返回上下文中保存的请求信息。
func FromContext(ctx context.Context) (Info, bool) {
	info, ok := ctx.Value(contextKey{}).(Info)
	return info, ok
}
