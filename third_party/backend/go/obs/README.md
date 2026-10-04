# Go obs

Matrix Go 后端统一观测包，提供结构化日志、OTel tracer、Gin 中间件和
lane-aware HTTP client。

## 环境变量

| env | 作用 | 缺省 |
|---|---|---|
| `OTEL_SERVICE_NAME` | 服务名 | `NACOS_DATA_ID` > `mosi-matrix` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTel Collector gRPC 地址 | 空则禁用 tracer |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` | `info` |
| `LOG_FORMAT` | `json` / `text` | `json` |
| `LOG_MAX_LINE_BYTES` | 单行日志最大字节数，最低有效值 `512` | `4096` |

`LOG_MAX_LINE_BYTES` 是 obs 层最后兜底。超限时日志仍保持可采集格式：

- `LOG_FORMAT=json` 输出仍是合法 JSON；
- 保留 `service`、`module`、`trace_id`、`span_id`、`lane`、`tenant_id`、
  `user_id` 等排障字段；
- 添加 `log_truncated=true`、`log_original_bytes`、`log_max_bytes`。

常规部署建议保持默认 `4096`。确需调整时可以在 SDK 层显式配置：

```go
obs.Init(obs.Config{
    Service:      "api-gateway",
    MaxLineBytes: 8192,
})
```

也可以通过 `LOG_MAX_LINE_BYTES` 按部署配置。业务代码仍应优先避免打印原始 body、
base64、token 或大对象。
