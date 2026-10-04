// Package nacos 封装 Nacos 配置中心 SDK，提供统一的配置加载和热更新能力。
//
// 使用方式：
//
//	type MyConfig struct {
//	    Server struct { Port string `yaml:"port"` } `yaml:"server"`
//	    MySQL  struct { DSN  string `yaml:"dsn"`  } `yaml:"mysql"`
//	}
//
//	var cfg MyConfig
//	client, err := nacos.Load(&cfg)
//	// cfg 已填充 Nacos 中的配置
//	// 配置变更时自动热更新 cfg
package nacos

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"gopkg.in/yaml.v3"
)

// isNotExistErr 判断 nacos client 返回的 "DataID 不存在" 错误。
// SDK 没有导出 sentinel error，只能字符串匹配（v2.x 服务端返 "config data not exist"）。
func isNotExistErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "config data not exist")
}

// Options 配置选项，全部从环境变量读取，可通过 WithXxx 覆盖。
type Options struct {
	Host      string
	Port      uint64
	Namespace string
	DataID    string
	Group     string
	Username  string
	Password  string
	LogDir    string
	CacheDir  string
}

// Option 函数选项
type Option func(*Options)

func WithHost(h string) Option      { return func(o *Options) { o.Host = h } }
func WithPort(p uint64) Option      { return func(o *Options) { o.Port = p } }
func WithNamespace(n string) Option { return func(o *Options) { o.Namespace = n } }
func WithDataID(d string) Option    { return func(o *Options) { o.DataID = d } }
func WithGroup(g string) Option     { return func(o *Options) { o.Group = g } }
func WithUsername(u string) Option  { return func(o *Options) { o.Username = u } }
func WithPassword(p string) Option  { return func(o *Options) { o.Password = p } }

// Client 封装 Nacos 配置客户端
type Client struct {
	client  config_client.IConfigClient
	opts    Options
	mu      sync.RWMutex
	target  any
	onChange func()
}

// defaultOptions 从环境变量构建默认配置
func defaultOptions() Options {
	return Options{
		Host:      os.Getenv("NACOS_HOST"),
		Port:      8848,
		Namespace: os.Getenv("NACOS_NAMESPACE"),
		DataID:    EnvOr("NACOS_DATA_ID", ""),
		Group:     EnvOr("NACOS_GROUP", "matrix"),
		Username:  EnvOr("NACOS_USERNAME", "nacos"),
		Password:  EnvOr("NACOS_PASSWORD", "nacos"),
		LogDir:    "/tmp/nacos/log",
		CacheDir:  "/tmp/nacos/cache",
	}
}

// Enabled 检查是否配置了 NACOS_HOST 环境变量
func Enabled() bool {
	return os.Getenv("NACOS_HOST") != ""
}

// Load 从 Nacos 加载配置到 target（必须是指针）。
// 成功返回 Client，可用于后续操作。配置变更时自动热更新 target。
//
// 如果 NACOS_HOST 未设置，返回 nil, nil（表示未启用 Nacos）。
func Load(target any, opts ...Option) (*Client, error) {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}

	if o.Host == "" {
		return nil, nil
	}

	if o.DataID == "" {
		return nil, fmt.Errorf("nacos: NACOS_DATA_ID is required")
	}

	sc := []constant.ServerConfig{
		*constant.NewServerConfig(o.Host, o.Port),
	}
	cc := constant.NewClientConfig(
		constant.WithNamespaceId(o.Namespace),
		constant.WithTimeoutMs(5000),
		constant.WithLogLevel("warn"),
		constant.WithUsername(o.Username),
		constant.WithPassword(o.Password),
		constant.WithLogDir(o.LogDir),
		constant.WithCacheDir(o.CacheDir),
	)

	configClient, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  cc,
		ServerConfigs: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("nacos: create client error: %w", err)
	}

	// === Overlay 模式 ===
	// 泳道配置隔离用 base + overlay deep-merge 模式（不是全量复制）：
	//   NACOS_BASE_DATA_ID env 等于 NACOS_DATA_ID（或不设）→ Base Pod，单读 NACOS_DATA_ID
	//   NACOS_BASE_DATA_ID != NACOS_DATA_ID                 → Lane Pod，读 base + overlay 深合并
	//
	// overlay 不存在或空 → 退化为 base（lane 没改任何 key 时的默认状态）。
	// 基准 Pod 完全不会感知此机制——env 没设 NACOS_BASE_DATA_ID，行为跟之前一样。
	baseDataID := os.Getenv("NACOS_BASE_DATA_ID")
	isLane := baseDataID != "" && baseDataID != o.DataID

	content, err := loadAndMaybeMerge(configClient, baseDataID, o.DataID, o.Group, isLane)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal([]byte(content), target); err != nil {
		return nil, fmt.Errorf("nacos: parse config error: %w", err)
	}

	c := &Client{
		client: configClient,
		opts:   o,
		target: target,
	}

	// 热更新监听
	if isLane {
		// Lane Pod：监听 base + overlay 两个 DataID，任一变重新拉两个 merge
		c.listenLaneOverlay(baseDataID)
	} else {
		// Base Pod：单 DataID 监听
		c.listenSingle(o.DataID, o.Group)
	}

	if isLane {
		log.Printf("nacos: lane config loaded (base=%s overlay=%s group=%s)", baseDataID, o.DataID, o.Group)
	} else {
		log.Printf("nacos: config loaded from %s (dataId=%s, group=%s)", o.Host, o.DataID, o.Group)
	}
	return c, nil
}

// loadAndMaybeMerge 读 base（必须）+ overlay（可选），按需深合并。
// isLane=false 时只读 dataID（行为同原版）。
func loadAndMaybeMerge(client config_client.IConfigClient, baseDataID, dataID, group string, isLane bool) (string, error) {
	if !isLane {
		c, err := client.GetConfig(vo.ConfigParam{DataId: dataID, Group: group})
		if err != nil {
			return "", fmt.Errorf("nacos: get config error: %w", err)
		}
		if c == "" {
			return "", fmt.Errorf("nacos: config is empty (dataId=%s, group=%s)", dataID, group)
		}
		return c, nil
	}
	// Lane: 必须有 base
	base, err := client.GetConfig(vo.ConfigParam{DataId: baseDataID, Group: group})
	if err != nil {
		return "", fmt.Errorf("nacos: get base config error: %w", err)
	}
	if base == "" {
		return "", fmt.Errorf("nacos: base config is empty (dataId=%s, group=%s)", baseDataID, group)
	}
	// Overlay 可空：DataID 整个不存在 → 当空 overlay，返 base。
	// 用于 lane 自动 copy 还没跑（老分支 / webhook 漏触发）的情况，避免 lane Pod 启动崩。
	// 其它错误（网络/鉴权）仍返 error。
	overlay, err := client.GetConfig(vo.ConfigParam{DataId: dataID, Group: group})
	if err != nil {
		if isNotExistErr(err) {
			log.Printf("nacos: lane overlay not found (dataId=%s, group=%s) — fallback to base only", dataID, group)
			return base, nil
		}
		return "", fmt.Errorf("nacos: get overlay config error: %w", err)
	}
	return deepMergeYAML(base, overlay)
}

// deepMergeYAML 深合并两份 YAML 字符串（lane overlay 优先于 base）。
// overlay 为空则直接返回 base。
func deepMergeYAML(baseYAML, overlayYAML string) (string, error) {
	if overlayYAML == "" {
		return baseYAML, nil
	}
	var base, overlay map[string]any
	if err := yaml.Unmarshal([]byte(baseYAML), &base); err != nil {
		return "", fmt.Errorf("parse base yaml: %w", err)
	}
	if err := yaml.Unmarshal([]byte(overlayYAML), &overlay); err != nil {
		return "", fmt.Errorf("parse overlay yaml: %w", err)
	}
	merged := deepMergeMap(base, overlay)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("marshal merged yaml: %w", err)
	}
	return string(out), nil
}

// deepMergeMap base 和 overlay 都不可为 nil 调用者已处理。
// 规则（Helm values 模式）：
//   - 都是 map：递归深合并
//   - list / scalar：overlay 整个替换 base
//   - overlay 没有的 key：保留 base 值
func deepMergeMap(base, overlay map[string]any) map[string]any {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay
	}
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if bv, ok := out[k]; ok {
			if bm, bIsMap := bv.(map[string]any); bIsMap {
				if vm, vIsMap := v.(map[string]any); vIsMap {
					out[k] = deepMergeMap(bm, vm)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

// listenSingle 监听单 DataID（base Pod 用）。
func (c *Client) listenSingle(dataID, group string) {
	c.client.ListenConfig(vo.ConfigParam{
		DataId: dataID,
		Group:  group,
		OnChange: func(namespace, group, dataId, data string) {
			c.applyHotReload(data, dataId)
		},
	})
}

// listenLaneOverlay lane Pod 监听 base + overlay，任一变重新 merge。
func (c *Client) listenLaneOverlay(baseDataID string) {
	rebuild := func() {
		merged, err := loadAndMaybeMerge(c.client, baseDataID, c.opts.DataID, c.opts.Group, true)
		if err != nil {
			log.Printf("nacos: hot-reload re-merge error: %v", err)
			return
		}
		c.applyHotReload(merged, fmt.Sprintf("merged(base=%s,overlay=%s)", baseDataID, c.opts.DataID))
	}
	c.client.ListenConfig(vo.ConfigParam{
		DataId: baseDataID, Group: c.opts.Group,
		OnChange: func(namespace, group, dataId, data string) { rebuild() },
	})
	c.client.ListenConfig(vo.ConfigParam{
		DataId: c.opts.DataID, Group: c.opts.Group,
		OnChange: func(namespace, group, dataId, data string) { rebuild() },
	})
}

func (c *Client) applyHotReload(data, label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := yaml.Unmarshal([]byte(data), c.target); err != nil {
		log.Printf("nacos: hot-reload parse error: %v", err)
		return
	}
	log.Printf("nacos: config hot-reloaded (%s)", label)
	if c.onChange != nil {
		c.onChange()
	}
}

// OnChange 注册配置变更回调
func (c *Client) OnChange(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChange = fn
}

// RLock/RUnlock 用于安全读取热更新的配置
func (c *Client) RLock()   { c.mu.RLock() }
func (c *Client) RUnlock() { c.mu.RUnlock() }

// EnvOr 读取环境变量，不存在时返回 fallback。
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
