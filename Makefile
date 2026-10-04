GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

.PHONY: init
# 初始化开发工具。
init:
	GOWORK=off go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	GOWORK=off go install github.com/go-kratos/kratos/cmd/kratos/v3@latest
	GOWORK=off go install github.com/google/wire/cmd/wire@latest
	GOWORK=off go install github.com/bufbuild/buf/cmd/buf@latest

.PHONY: config
# 生成配置相关 Proto 代码。
config:
	GOWORK=off buf generate --template buf.gen.config.yaml

.PHONY: api
# 生成 API Proto 代码。
api:
	GOWORK=off buf generate --template buf.gen.yaml

.PHONY: sqlc
# 生成数据库访问代码。
sqlc:
	sqlc generate

.PHONY: build
# 构建两个业务进程。
build:
	mkdir -p bin/
	GOWORK=off go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/workflow ./cmd/workflow
	GOWORK=off go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/worker ./cmd/worker

.PHONY: local-maas
# 启动本地 MaaS Mock；可通过 MOCK_MAAS_MODE=failed 模拟失败。
local-maas:
	GOWORK=off go run ./cmd/mock-maas

.PHONY: local-up
# 启动本地 PostgreSQL、Temporal、Temporal UI 和 MaaS Mock。
local-up:
	docker compose -f docker-compose.local.yaml up -d --wait postgres temporal temporal-ui maas-mock

.PHONY: local-start
# 构建并启动完整本地 Docker 环境。
local-start:
	docker compose -f docker-compose.local.yaml up -d --build --wait

.PHONY: local-down
# 停止本地全部服务；保留 PostgreSQL 数据卷。
local-down:
	docker compose -f docker-compose.local.yaml down

.PHONY: local-build
# 构建本地 Docker 镜像。
local-build:
	docker compose -f docker-compose.local.yaml build

.PHONY: local-run
# 使用本机配置启动 Workflow API。
local-run:
	NACOS_HOST= LANE= OTEL_EXPORTER_OTLP_ENDPOINT= PG_DSN='postgres://workflow:workflow@127.0.0.1:15432/workflow?sslmode=disable&application_name=workflow' GOWORK=off go run ./cmd/workflow -conf ./configs/config.local.yaml

.PHONY: local-run-worker
# 使用本机配置启动 Worker。
local-run-worker:
	NACOS_HOST= LANE= OTEL_EXPORTER_OTLP_ENDPOINT= PG_DSN='postgres://workflow:workflow@127.0.0.1:15432/workflow?sslmode=disable&application_name=worker' GOWORK=off go run ./cmd/worker -conf ./configs/config.local.worker.yaml

.PHONY: local-test
# 运行项目测试。
local-test:
	GOWORK=off go test ./...

.PHONY: generate
# 执行代码生成和依赖整理。
generate:
	GOWORK=off go generate ./...
	GOWORK=off go mod tidy

.PHONY: all
# 执行代码生成和依赖整理。
all:
	make api
	make config
	make generate

# 显示帮助信息。
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
