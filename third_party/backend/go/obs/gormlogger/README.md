# obs/gormlogger

GORM 的 `logger.Interface` 适配器，把 SQL 错误 / 慢查询 / 普通查询桥接到
[`obs`](../) 的结构化 JSON 日志。trace_id / lane / tenant_id 通过 `ctx`
自动注入，跟现有可观测性链路打通。

独立子 module 的原因：`gorm.io/gorm` 是个有内容量的依赖，放主 obs
包会让 25+ 个 workspace module 的 go.mod 都被加上 `gorm // indirect`。
拆出来后，**只有真正 import 这个适配器的服务会引入 gorm**。

## 接入

### 最简

```go
import (
    "git.sotatts.online/matrix/matrix/packages/backend/go/obs/gormlogger"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
)

db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: gormlogger.New(),
})
```

**前提**：服务 `main` 里调过 `obs.Init(...)`。绝大多数服务都已经调过；
没调日志会兜底走 fallback 但 `trace_id` 会丢。

### 调阈值 / 多 DB 区分

```go
import (
    "time"

    "git.sotatts.online/matrix/matrix/packages/backend/go/obs/gormlogger"
    gormlog "gorm.io/gorm/logger"
)

db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: gormlogger.New(
        gormlogger.SlowThreshold(500 * time.Millisecond),  // 默认 200ms
        gormlogger.IgnoreRecordNotFound(true),             // 默认 true
        gormlogger.Module("ai-asset-db"),                  // 多 DB 区分；默认 "gorm"
        gormlogger.LogLevel(gormlog.Warn),                 // 默认 Warn
    ),
})
```

## 行为

每条 SQL 执行后 GORM 调一次 `Trace`，按下面优先级走：

| 场景 | 走哪条路径 | JSON 字段 |
|---|---|---|
| 真错误（非 `RecordNotFound`） | `obs.ErrorContext` | `sql / rows / elapsed_ms / error` |
| 慢查询（默认 > 200ms） | `obs.WarnContext` | `sql / rows / elapsed_ms / slow_threshold_ms` |
| 正常 SQL（仅 `LogLevel ≥ Info`） | `obs.DebugContext` | `sql / rows / elapsed_ms` |

GORM 自身的 `Info / Warn / Error` 直调（用于框架启动、钩子警告等）会
转成对应 `obs.*Context` 调用，仅在调用方传了 args 才走 `fmt.Sprintf`，
避免动态 SQL 含 `%` 时被当格式串解释。

### LogLevel 含义

GORM 原生四档：

- `Silent` — 全静音，连真错误都不打
- `Error` — 只打错误
- `Warn` — 错误 + 慢查询 ✅ **prod 默认**
- `Info` — 全打（含正常 SQL，用 Debug 级别避免刷屏）

### `RecordNotFound` 默认静音

业务侧 `.First() / .Take()` 没拿到记录是高频"错误"，跟 SQL 真炸了
（`Unknown column` / 死锁 / 连接断）混在一起会淹没排障，所以默认静音。
如果业务确实把它当严重错误，传 `gormlogger.IgnoreRecordNotFound(false)`。

## Loki 怎么查

| 想看啥 | LogQL |
|---|---|
| 你服务的所有 SQL 错误 | `{service="<svc>"} \| json \| module="gorm" \| level="error"` |
| 慢查询 | `{service="<svc>"} \| json \| module="gorm" \| level="warn" \| msg="gorm slow query"` |
| 某条 trace 的所有 SQL | `{service="<svc>"} \| json \| module="gorm" \| trace_id="<id>"` |
| 跨服务找慢查询 top | `{job="matrix"} \| json \| msg="gorm slow query" \| line_format "{{.elapsed_ms}}ms {{.service}} {{.sql}}"` |

字段 schema：`module / level / sql / rows / elapsed_ms / error / slow_threshold_ms / trace_id / lane / tenant_id / service`。

## 常见坑

1. **`.First()` / `.Take()` 没命中不会报 error log** — `ErrRecordNotFound`
   默认静音。业务层判这个错自己处理；想看就 `gormlogger.IgnoreRecordNotFound(false)`。
2. **`obs.Init` 没调** → trace_id / span_id 会丢，stderr 会红字 warning。
   生产服务 `main` 第一行调它。
3. **多 DB 实例** → 不传 `Module(...)` 都叫 `"gorm"`，排障分不清，一律
   传一个有意义的名字（`"ai-asset-db"` / `"billing-db"` 之类）。
4. **prod 不要开 `LogLevel(Info)`** —— 会把所有正常 SQL 都打到 Debug 流，
   配合 `LOG_LEVEL=debug` 时会刷屏；本地开发用没问题。
