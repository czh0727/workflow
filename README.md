# Kratos 工作流服务本地学习项目

这是从公司 `workflow/api-server` 迁出的本地学习版本。项目保留原有的 API、业务分层、Temporal 工作流、数据库访问和依赖注入方式；公司内部不可用的 MaaS API Gateway 改为本地 `maas-mock`，便于在没有公司网络和凭据的环境中学习、调试和测试。

## 本地运行

### 端口

| 服务 | 宿主机端口 | 容器内端口或用途 |
| --- | ---: | --- |
| Workflow API | 8080 | HTTP |
| Worker API | 8081 | HTTP |
| PostgreSQL | **15432** | 容器内 5432 |
| Temporal | 17233 | 容器内 7233 |
| MaaS Mock | 18081 | HTTP |
| Temporal UI | 18082 | Web UI |

PostgreSQL 的宿主机端口特意设置为 `15432`，避免占用本机已有的 `5432`。宿主机上的 Go 进程使用 `127.0.0.1:15432`；Compose 内的服务必须使用 `postgres:5432`。

### 启动完整 Docker 环境

```bash
make local-start
```

`local-start` 会构建并启动 PostgreSQL、Temporal、Temporal UI、MaaS Mock、Workflow API 和 Worker。Workflow 与 Worker 会等待 PostgreSQL、Temporal 和 MaaS Mock 的健康检查通过后再启动。

检查状态：

```bash
docker compose -f docker-compose.local.yaml ps
curl http://127.0.0.1:8080/live
curl http://127.0.0.1:8081/live
curl http://127.0.0.1:18081/healthz
```

Temporal UI 地址为 <http://127.0.0.1:18082>。

停止服务并保留数据库数据：

```bash
make local-down
```

只有需要重新初始化数据库时才删除数据卷：

```bash
docker compose -f docker-compose.local.yaml down -v
```

首次创建数据卷时，PostgreSQL 会创建 `temporal`、`temporal_visibility` 和 `workflow` 数据库，并执行 `internal/data/db/migrations/000001_init.sql`。业务表的初始化文件直接复用项目迁移文件，避免 Docker 脚本和正式迁移各维护一份结构。

### 在宿主机运行 Go 进程

先启动 PostgreSQL、Temporal、Temporal UI 和 MaaS Mock：

```bash
make local-up
```

如果完整 Docker 环境中的 Workflow 和 Worker 正在运行，先执行 `docker compose -f docker-compose.local.yaml stop workflow worker` 释放 8080/8081 端口。再分别在两个终端中启动 Go 进程：

```bash
make local-run
make local-run-worker
```

宿主机配置在 `configs/config.local.yaml` 和 `configs/config.local.worker.yaml`。两个配置使用本地 Temporal 地址 `127.0.0.1:17233`、MaaS Mock 地址 `http://127.0.0.1:18081`，并通过 `PG_DSN` 连接 `127.0.0.1:15432`。

如果只需要单独调试 MaaS Mock：

```bash
make local-maas
```

不要在 Compose 已经启动 `maas-mock` 时再次执行这个命令，否则会争用 `18081` 端口。

## MaaS Mock 行为

Mock 实现位于 `cmd/mock-maas`，保留原客户端需要的提交和查询任务协议：

- `POST /internal/v2/image_generation`
- `POST /internal/v2/video_generation`
- `POST /internal/v2/tts`
- `POST /internal/v2/ttsd`
- `POST /internal/v2/voice_generator`
- `POST /internal/v2/voice_convert`
- `POST /internal/v2/speech_enhance`
- `POST /internal/v2/sound_effect_generation`
- `POST /internal/v2/audio_transcription`
- `POST /internal/v2/transcription_diarization`
- `GET /internal/v2/tasks/{task_id}`

默认提交后直接返回成功。为了测试轮询和失败分支，可以设置：

```bash
MOCK_MAAS_PROCESSING_POLLS=2 MOCK_MAAS_MODE=success make local-maas
MOCK_MAAS_MODE=failed make local-maas
```

Mock 返回的音频、图片和视频 URL 是协议占位值，用于验证工作流节点的数据传递，不代表真实模型产物；源项目本身也不会在这些节点中下载媒体文件。

## API 验证示例

创建一个最小工作流：

```bash
curl -sS -X POST http://127.0.0.1:8080/internal/v1/workflows \
  -H 'Content-Type: application/json' \
  -d '{"uid":1,"name":"本地示例","type":"template","definition":{"nodes":[{"id":"say","type":"SAY_SOMETHING","preset":{"prompt":"hello"}},{"id":"output","type":"OUTPUT","preset":{}}],"edges":[{"from_node":"say","from_port":"text","to_node":"output","to_input":"text"}]}}'
```

使用返回的 `workflow_id` 创建执行，再通过 `GET /internal/v1/executions/{execution_id}` 轮询状态。执行完成后状态为小写的 `completed`，结果放在 `data.output` 中。

查看支持的节点：

```bash
curl -sS http://127.0.0.1:8080/internal/v1/node-types
```

健康检查和指标：

```text
GET /live
GET /ready
GET /metrics
```

Workflow API 与 Worker 都提供这些 HTTP 端点；Worker 仍然只监听 HTTP，gRPC 代码和配置保留为未来扩展占位，当前不会监听 9000/9001。

## 配置规则

设置 `NACOS_HOST` 时，两个进程会从 Nacos 读取 `dataId=api-server`、`group=workflow` 的配置，可用 `NACOS_DATA_ID` 和 `NACOS_GROUP` 覆盖。未设置 `NACOS_HOST` 时，通过 `-conf` 读取文件或目录。本地目录中包含多套配置，启动时应明确选择单个文件，避免合并到默认的公司 `api-gateway` 地址。

数据库连接必须通过环境变量 `PG_DSN` 提供。Docker Compose 使用容器内地址：

```text
postgres://workflow:workflow@postgres:5432/workflow?sslmode=disable&application_name=workflow
```

宿主机 Go 进程使用：

```text
postgres://workflow:workflow@127.0.0.1:15432/workflow?sslmode=disable&application_name=workflow
```

本地配置中的 `maas_api_gateway.service_token` 保持为空，`base_url` 指向 MaaS Mock；不需要公司 Token、Nacos 或内部观测平台即可运行。未设置 OTLP 追踪地址时不会向公司环境发送追踪数据。

## 项目结构

```text
api/                  Proto 源文件和生成的 API 绑定
cmd/                  Workflow、Worker 和 MaaS Mock 入口
configs/               默认配置和本地/Docker 配置
internal/conf/         配置 Proto 及生成代码
internal/server/       HTTP、gRPC 和 Temporal Worker 组装
internal/service/      DTO 与领域模型之间的传输适配
internal/biz/          领域模型、用例、错误和仓储接口
internal/data/         PostgreSQL 仓储和外部客户端实现
internal/data/db/      迁移、查询和 sqlc 生成代码
pkg/                   日志、指标、请求上下文等公共适配器
third_party/           从公司 monorepo 迁入的本地 Go 依赖
openapi.yaml           生成的 OpenAPI 文档
docker/                PostgreSQL 和 Temporal 本地初始化文件
```

代码仍保持 `service → biz → data` 的依赖方向：service 只负责 DTO↔DO 转换，biz 持有业务模型和仓储接口，data 实现仓储并负责 DO↔PO 转换，cmd 通过 Wire 组装全部层。这个边界用于学习和后续优化时定位改动范围。

## 生成代码和开发命令

生成器已经写入项目原有配置，不要手工编辑生成文件：

```bash
make api       # 生成 Proto、gRPC、HTTP 和 OpenAPI 代码
make config    # 生成配置 Proto 代码
make sqlc      # 生成数据库访问代码
make generate  # 执行 go generate 和 go mod tidy
make all       # 执行 API、配置和其他生成步骤
```

构建和测试：

```bash
make build
GOWORK=off go test ./...
```

项目通过 `replace` 将公司内部的 `nacos`、`httpclient` 和 `obs` 模块指向 `third_party/backend/go`，不需要访问公司的 monorepo。源项目的模块路径和包导入保持不变，因此业务代码的目录和逻辑无需改写。

## 原始模板说明

该服务基于 Kratos 模板，包含 Protobuf-first API、HTTP 传输、保留的 gRPC 生成代码、Wire 依赖注入、OpenAPI 生成、分层的 service/biz/data 包和 Todo 示例。新增资源时，先在 `api/<domain>/v1/` 编写 Proto，再运行 `make api`，然后依次补充 biz 模型与仓储接口、data 实现、service DTO 转换和 server 注册，最后运行测试。

生成的 `*.pb.go`、`*_grpc.pb.go`、`*_http.pb.go`、`openapi.yaml`、sqlc 输出和 `wire_gen.go` 只能通过对应生成命令更新。数据库查询放在 `internal/data/db/queries`，迁移放在 `internal/data/db/migrations`，不要在业务层引用 sqlc 类型或 SQL 行结构。
