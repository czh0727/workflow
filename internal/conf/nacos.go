package conf

import (
	"bytes"
	"context"
	"errors"
	"sync"

	sharednacos "git.sotatts.online/matrix/matrix/packages/backend/go/nacos"
	"github.com/go-kratos/kratos/v3/config"
	"gopkg.in/yaml.v3"
)

type nacosSource struct {
	dataID  string
	group   string
	config  rawYAML
	updates chan struct{}
	client  *sharednacos.Client
}

// NewNacosSource 将共享 Nacos 客户端适配为 Kratos 配置源。
func NewNacosSource() config.Source {
	return &nacosSource{
		dataID:  sharednacos.EnvOr("NACOS_DATA_ID", "api-server"),
		group:   sharednacos.EnvOr("NACOS_GROUP", "workflow"),
		updates: make(chan struct{}, 1),
	}
}

func (s *nacosSource) Load() ([]*config.KeyValue, error) {
	client, err := sharednacos.Load(
		&s.config,
		sharednacos.WithDataID(s.dataID),
		sharednacos.WithGroup(s.group),
	)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("nacos: source is not enabled")
	}
	s.client = client
	s.client.OnChange(func() {
		s.publish()
	})
	return s.keyValues(s.config.Bytes()), nil
}

func (s *nacosSource) Watch() (config.Watcher, error) {
	if s.client == nil {
		return nil, errors.New("nacos: source must be loaded before watching")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &nacosWatcher{
		ctx:     ctx,
		cancel:  cancel,
		source:  s,
		updates: s.updates,
	}, nil
}

func (s *nacosSource) publish() {
	select {
	case s.updates <- struct{}{}:
	default:
	}
}

func (s *nacosSource) keyValues(value []byte) []*config.KeyValue {
	return []*config.KeyValue{{
		Key:    s.dataID,
		Value:  value,
		Format: "yaml",
	}}
}

type nacosWatcher struct {
	ctx     context.Context
	cancel  context.CancelFunc
	source  *nacosSource
	updates <-chan struct{}
}

func (w *nacosWatcher) Next() ([]*config.KeyValue, error) {
	select {
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	case <-w.updates:
		return w.source.keyValues(w.source.config.Bytes()), nil
	}
}

func (w *nacosWatcher) Stop() error {
	w.cancel()
	return nil
}

type rawYAML struct {
	mu    sync.RWMutex
	value []byte
}

func (c *rawYAML) UnmarshalYAML(node *yaml.Node) error {
	value, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.value = value
	c.mu.Unlock()
	return nil
}

func (c *rawYAML) Bytes() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return bytes.Clone(c.value)
}
