package server

import (
	"github.com/google/wire"
)

// ProviderSet 包含 Workflow 应用启用的服务。
// NewGRPCServer 保留在 grpc.go 中，但当前不会注入。
var ProviderSet = wire.NewSet(NewHTTPServer)

// WorkerProviderSet 包含 Worker 应用启用的服务。
// NewWorkerGRPCServer 保留在 grpc.go 中，但当前不会注入。
var WorkerProviderSet = wire.NewSet(NewWorkerHTTPServer, NewTemporalWorker)
