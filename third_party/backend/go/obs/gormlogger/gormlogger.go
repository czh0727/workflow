// Package gormlogger 把 GORM 的 logger.Interface 桥接到 obs 的结构化 JSON 日志，
// 自动带 ctx 里的 trace_id / lane / tenant_id。
//
// 单独子 module 的原因：obs 自己不带 gorm 依赖；只有真正用 GORM 的服务 import
// 这个子包，gorm 才会出现在该服务的 go.mod 里，避免污染其他 25+ 个 workspace
// module。
//
// 业务里挂法：
//
//	import "git.sotatts.online/matrix/matrix/packages/backend/go/obs/gormlogger"
//
//	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
//	    Logger: gormlogger.New(),
//	})
//
// 想调阈值 / 静音 RecordNotFound：
//
//	gormlogger.New(
//	    gormlogger.SlowThreshold(500*time.Millisecond),
//	    gormlogger.IgnoreRecordNotFound(false),  // 默认 true
//	    gormlogger.Module("ai-asset-db"),        // 默认 "gorm"
//	)
//
// 字段约定（Loki 里直接用）：
//   - 错误：level=error msg="gorm error" sql=... rows=... elapsed_ms=... error=...
//   - 慢查询：level=warn msg="gorm slow query" sql=... rows=... elapsed_ms=... slow_threshold_ms=...
//   - 正常（仅当 LogLevel >= Info 时打）：level=debug msg="gorm trace" sql=... rows=... elapsed_ms=...
package gormlogger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"git.sotatts.online/matrix/matrix/packages/backend/go/obs"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

// Option 函数式选项，避免外部直接 struct literal 强耦合内部字段。
type Option func(*logger)

// SlowThreshold 慢查询阈值（默认 200ms，跟 GORM 官方默认一致）。
func SlowThreshold(d time.Duration) Option {
	return func(l *logger) { l.slowThreshold = d }
}

// IgnoreRecordNotFound 是否忽略 ErrRecordNotFound（默认 true）。
//
// 业务侧 .First() 没拿到记录是高频"错误"但通常不该报警，跟 SQL 真正炸了
// （Unknown column / 死锁 / 连接断）混在一起会淹没排障。
func IgnoreRecordNotFound(ignore bool) Option {
	return func(l *logger) { l.ignoreRecordNotFound = ignore }
}

// Module 给 logger 打个 module 字段（默认 "gorm"）。多个 DB 实例
// 用不同 module 区分。
func Module(name string) Option {
	return func(l *logger) { l.module = name }
}

// LogLevel 设置日志级别（默认 Warn — 只打 error + 慢查询，不刷正常 SQL）。
//
// LogLevel 含义（GORM 原生）：
//   - Silent: 全静音
//   - Error:  只打 error
//   - Warn:   error + 慢查询（推荐 prod 默认）
//   - Info:   全打（含正常 SQL，用 Debug 级别避免刷屏）
func LogLevel(level gormlog.LogLevel) Option {
	return func(l *logger) { l.level = level }
}

// New 构造一个 GORM logger.Interface 实现，写到 obs 的结构化日志。
func New(opts ...Option) gormlog.Interface {
	l := &logger{
		level:                gormlog.Warn,
		slowThreshold:        200 * time.Millisecond,
		ignoreRecordNotFound: true,
		module:               "gorm",
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

type logger struct {
	level                gormlog.LogLevel
	slowThreshold        time.Duration
	ignoreRecordNotFound bool
	module               string
}

// LogMode 按 GORM 接口约定返回**带新 level 的副本**，不能就地改 —— GORM 内部
// 在 Session 里复用 logger 时依赖这个不可变性。
func (l *logger) LogMode(level gormlog.LogLevel) gormlog.Interface {
	cp := *l
	cp.level = level
	return &cp
}

// formatMsg 仅在调用方真传了 args 才走 fmt.Sprintf —— GORM 的 logger.Interface
// 允许任意 msg 字符串（动态 SQL 经常含孤立的 %），无条件 Sprintf 会输出
// "%!(NOVERB)" 之类的脏日志或者掩盖真正消息。日志适配器不该把一次 SQL 操作
// 放大成日志污染。
func formatMsg(msg string, args []interface{}) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

func (l *logger) Info(ctx context.Context, msg string, args ...interface{}) {
	if l.level < gormlog.Info {
		return
	}
	obs.WithModule(l.module).InfoContext(ctx, formatMsg(msg, args))
}

func (l *logger) Warn(ctx context.Context, msg string, args ...interface{}) {
	if l.level < gormlog.Warn {
		return
	}
	obs.WithModule(l.module).WarnContext(ctx, formatMsg(msg, args))
}

func (l *logger) Error(ctx context.Context, msg string, args ...interface{}) {
	if l.level < gormlog.Error {
		return
	}
	obs.WithModule(l.module).ErrorContext(ctx, formatMsg(msg, args))
}

// Trace 按 GORM 规范每条 SQL 执行后被调用一次。三个分支按优先级判断：
//  1. 真错误（非 RecordNotFound 或调用方关了 ignore）→ ErrorContext
//  2. 慢查询（超阈值）→ WarnContext
//  3. 正常 SQL（仅 LogLevel >= Info 时）→ DebugContext，避免业务 prod 刷屏
func (l *logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlog.Silent {
		return
	}

	elapsed := time.Since(begin)
	log := obs.WithModule(l.module)

	switch {
	case err != nil && l.level >= gormlog.Error &&
		!(l.ignoreRecordNotFound && errors.Is(err, gorm.ErrRecordNotFound)):
		sql, rows := fc()
		log.ErrorContext(ctx, "gorm error",
			"sql", sql,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
			"error", err.Error(),
		)

	case elapsed > l.slowThreshold && l.slowThreshold > 0 && l.level >= gormlog.Warn:
		sql, rows := fc()
		log.WarnContext(ctx, "gorm slow query",
			"sql", sql,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
			"slow_threshold_ms", l.slowThreshold.Milliseconds(),
		)

	case l.level >= gormlog.Info:
		sql, rows := fc()
		log.DebugContext(ctx, "gorm trace",
			"sql", sql,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
		)
	}
}
