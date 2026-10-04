# 仓库协作规范

这是一个 Kratos 服务模板。本文件规定了修改模板时必须遵守的分层约定。

## 项目结构

```
api/<domain>/<version>/   Proto 源文件和生成桩，定义公开接口。
cmd/<app>/                进程入口、Wire 注入器和 main.go。
configs/                  运行配置（config.yaml），不包含密钥。
internal/conf/            配置 Proto，通过 make config 生成。
internal/server/          HTTP/gRPC 服务组装。
internal/service/         传输适配器，每个资源一个文件。
internal/biz/             领域模型、用例、仓储接口和错误。
internal/data/            仓储实现和存储客户端。
```

## 分层与依赖规则

三种模型在三层之间流转。`biz` 拥有 DO，`data` 拥有 PO；
`service` 负责透传，并在边界处进行转换。

```
   client ──► DTO ──► service ──► DO ──► biz ──► DO ──► data ──► PO ──► storage
                                  ▲                ▲
                                  │ 声明           │ 实现
                                  └─── 仓储接口 ───┘

   DTO  数据传输对象 —— Proto 请求与响应。
   DO   领域对象     —— 纯业务模型，不含 Proto 和存储标签。
   PO   持久化对象   —— 由 data 持有的存储结构。
```

| 层      | 持有 | 边界使用的模型 | 禁止使用                |
|---------|------|----------------|-------------------------|
| service | —    | DTO ↔ DO       | PO、存储客户端          |
| biz     | DO   | DO             | DTO、PO、存储客户端     |
| data    | PO   | DO ↔ PO        | DTO                     |

- `service` 导入 `api/...`（DTO）和 `biz`（DO），禁止导入 `data`。
- `biz` 仅为错误原因枚举导入 `api/...`，禁止导入 `service` 或 `data`。
  在此声明仓储接口，作为依赖反转的边界。
- `data` 导入 `biz` 并实现仓储接口，禁止导入 `service` 或 DTO。
- `cmd` 是唯一通过 Wire 组装全部层的位置。

违反上述依赖方向属于分层错误，应修正设计，而不是增加反向导入。

### 各层职责

**service（DTO ↔ DO）**

- `convert<Resource>` 将输入 Proto 解析为 DO。反向转换直接在返回处构造；
  返回类型遵循 Proto 声明，通常为资源本身（`return &v1.<Resource>{...}, nil`），
  有时为列表包装类型（`*v1.<Resources>Set`），删除则返回 `&emptypb.Empty{}`。
  内联构造可让每个处理方法保持完整。
- 嵌入 `Unimplemented<Resource>ServiceServer`。
- 使用 `filtering` / `ordering` / `pagination` 解析 AIP 列表请求；
  使用 `fieldmask.Update` 处理部分更新。
- 在调用用例之前，于服务边界校验请求参数。
- 返回 `biz` 错误；不包含业务规则、存储访问或 PO。

**biz（仅 DO）**

- 持有 DO（`type <Resource> struct`，不含 Proto 和存储标签）、用例和
  仓储接口（`type <Resource>Repo interface`）。
- 使用 `errors.NotFound` / `errors.BadRequest` 和 API 错误原因枚举
  定义具有类型的错误。
- 持有 `ListOption` 辅助函数：`ListFilter`、`ListOrderBy`、`ListOffset`、
  `ListLimit`，使调用方可以组合查询，同时不暴露存储细节。

**data（DO ↔ PO）**

- _仓储结构_：实现 `biz.<Resource>Repo`。构造函数返回接口而非具体类型：
  `func New<Resource>Repo(d *Data) biz.<Resource>Repo`。
- _PO 与转换_：存储结构与 DO 不同时定义 PO。PO 保留在 `data` 内部。
  使用独立函数 `new<Resource>`（DO → PO，写入）和 `toBiz`（PO → DO，读取）
  转换。驱动专用的构建器类型不得离开 `data`。
- _共享客户端_：`*Data`（定义于 `internal/data/data.go`）持有长期使用的
  存储客户端。仓储接收 `*Data`，不自行创建客户端。
- _查询_：在仓储内部将 `ListOptions.Filter` 和 `ListOptions.OrderBy`
  转换为存储驱动的查询语言。
- _错误_：将驱动错误映射为 `biz` 错误，上层不得根据驱动类型做分支。

**server**

- 构造 HTTP/gRPC 服务、应用中间件并注册服务；不负责模型转换和业务逻辑。

### 新增资源检查清单

1. **DTO**：在 `api/<domain>/<version>/` 定义 `Create<Resource>` /
   `Get<Resource>` / `List<Resources>` / `Update<Resource>` / `Delete<Resource>`，
   然后执行 `make api`。
2. **DO 与仓储接口**：在 `biz` 中声明两者，并基于接口构建用例。
3. **仓储实现**：在 `data` 中实现并返回 `biz.<Resource>Repo`；存储结构与
   DO 不同时，增加 PO 及对应的转换函数。
4. **组装**：将仓储构造函数加入 `data.ProviderSet`，用例加入 `biz.ProviderSet`，
   服务加入 `service.ProviderSet`；在 `internal/server` 注册 HTTP/gRPC 服务。
5. **重新生成**：执行 `make all` 更新 Wire 和 `go.mod`。

### 测试边界

测试与被测代码放在一起（`*_test.go`）。各层独立测试：service 测试替换用例，
biz 测试替换仓储，data 测试在存储边界验证仓储实现。

## 代码生成与生成文件

使用 `make api`、`make config` 或 `make all` 重新生成；禁止手工编辑
`*.pb.go`、`*_grpc.pb.go`、`*_http.pb.go` 或 `wire_gen.go`。

## 命名与错误原因

- 资源：`<Resource>`（如 `Todo`）；集合 RPC：`List<Resources>`。
- 类型：仓储 `<Resource>Repo`，用例 `<Resource>Usecase`，服务 `<Resource>Service`。
  PO 类型放在 `internal/data/` 内，按存储驱动选取适合的名称，并通过独立函数
  `new<Resource>(do)` / `toBiz(po)` 转换。
- 错误原因：定义在 `api/<domain>/<version>/error_reason.proto`，并在 `biz`
  中暴露为 `Err<Resource><Cause>`。

## 提交与安全

- 遵循 Conventional Commits：`feat:`、`fix:`、`refactor:`、`chore(deps):`、
  `docs:`、`test:`。生成文件与其源文件在同一个提交中更新。
- 禁止在 `configs/config.yaml` 中提交真实凭据。
