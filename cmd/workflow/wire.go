//go:build wireinject
// +build wireinject

// 此构建标签确保依赖注入桩不会进入最终构建。

package main

import (
	"log/slog"

	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/biz"
	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/conf"
	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/data"
	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/server"
	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/service"
	"git.sotatts.online/matrix/matrix/workflow/api-server/pkg/metrics"

	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

// wireApp 初始化 Kratos 应用。
func wireApp(*conf.Server, *conf.Data, *slog.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		metrics.New,
		metrics.NewTemporalHandler,
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		newApp,
	))
}
