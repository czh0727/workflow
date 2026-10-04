package main

import (
	"flag"
	stdslog "log/slog"
	"os"
	"strings"

	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/conf"
	"git.sotatts.online/matrix/matrix/workflow/api-server/pkg/slog"

	sharednacos "git.sotatts.online/matrix/matrix/packages/backend/go/nacos"
	sharedobs "git.sotatts.online/matrix/matrix/packages/backend/go/obs"
	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/config/file"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

// 构建时可使用 go build -ldflags "-X main.Version=x.y.z" 注入版本。
var (
	// Name 是编译产物的名称。
	Name = "workflow"
	// Version 是编译产物的版本。
	Version string
	// flagconf 是配置文件路径参数。
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger *stdslog.Logger, hs *http.Server) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			hs,
		),
	)
}

func main() {
	flag.Parse()
	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = strings.TrimSpace(os.Getenv("NACOS_DATA_ID"))
	}
	if serviceName == "" {
		serviceName = Name
	}
	logger := slog.Init(id, serviceName, Version)
	if err := run(serviceName, logger); err != nil {
		os.Exit(1)
	}
}

func run(serviceName string, logger *stdslog.Logger) error {
	if err := sharedobs.Init(sharedobs.Config{Service: serviceName}); err != nil {
		logger.Error("application initialization failed", "stage", "observability", "error", err)
		return err
	}
	defer sharedobs.Close()

	source := config.Source(file.NewSource(flagconf))
	if sharednacos.Enabled() {
		source = conf.NewNacosSource()
	}
	c := config.New(config.WithSource(source, conf.NewEnvSource()))
	defer c.Close()

	if err := c.Load(); err != nil {
		logger.Error("application initialization failed", "stage", "config_load", "error", err)
		return err
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		logger.Error("application initialization failed", "stage", "config_scan", "error", err)
		return err
	}

	app, cleanup, err := wireApp(bc.Server, bc.Data, logger)
	if err != nil {
		logger.Error("application initialization failed", "stage", "dependency_init", "error", err)
		return err
	}
	defer cleanup()

	// 启动服务并等待停止信号。
	if err := app.Run(); err != nil {
		logger.Error("application exited", "error", err)
		return err
	}
	return nil
}
