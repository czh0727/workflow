package conf

import (
	"errors"
	"strings"

	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/config/env"
)

const (
	postgresDSNEnv = "PG_DSN"
	postgresDSNKey = "data.database.source"
)

type envSource struct {
	source config.Source
}

// NewEnvSource 将只存在于环境变量中的配置映射到 Kratos 配置树。
func NewEnvSource() config.Source {
	return &envSource{source: env.NewSource()}
}

func (s *envSource) Load() ([]*config.KeyValue, error) {
	values, err := s.source.Load()
	if err != nil {
		return nil, err
	}

	for _, value := range values {
		if value.Key != postgresDSNEnv {
			continue
		}
		dsn := strings.TrimSpace(string(value.Value))
		if dsn != "" {
			return []*config.KeyValue{{Key: postgresDSNKey, Value: []byte(dsn)}}, nil
		}
		break
	}
	return nil, errors.New("PG_DSN environment variable is required")
}

func (s *envSource) Watch() (config.Watcher, error) {
	return s.source.Watch()
}
