package nacos

import (
	"errors"
	"strings"
	"testing"
)

func TestIsNotExistErr(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"nil":                   {nil, false},
		"server data not exist": {errors.New("config data not exist"), true},
		"wrapped":               {errors.New("nacos: get overlay: config data not exist"), true},
		"network err":           {errors.New("dial tcp: connection refused"), false},
	}
	for name, c := range cases {
		if got := isNotExistErr(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", name, got, c.want)
		}
	}
}

func TestDeepMergeYAML_OverlayEmpty(t *testing.T) {
	base := "server:\n  port: 8080\n"
	got, err := deepMergeYAML(base, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != base {
		t.Fatalf("overlay 空应该原样返回 base，got: %q", got)
	}
}

func TestDeepMergeYAML_ScalarOverride(t *testing.T) {
	base := "server:\n  port: 8080\n  host: 0.0.0.0\n"
	overlay := "server:\n  port: 9090\n"
	got, err := deepMergeYAML(base, overlay)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// port 被 overlay 覆盖、host 保留
	if !strings.Contains(got, "port: 9090") {
		t.Fatalf("port 应被 overlay 覆盖为 9090，got: %s", got)
	}
	if !strings.Contains(got, "host: 0.0.0.0") {
		t.Fatalf("host 应保留 base，got: %s", got)
	}
}

func TestDeepMergeYAML_DeepMap(t *testing.T) {
	base := `
server:
  port: 8080
mysql:
  host: a
  port: 3306
  pool:
    max: 100
    idle: 10
`
	overlay := `
mysql:
  pool:
    max: 200
`
	got, err := deepMergeYAML(base, overlay)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// pool.max 覆盖、pool.idle 保留、mysql.host/port 保留、server 全保留
	for _, want := range []string{"max: 200", "idle: 10", "host: a", "port: 3306", "port: 8080"} {
		if !strings.Contains(got, want) {
			t.Fatalf("merged 应含 %q，got: %s", want, got)
		}
	}
}

func TestDeepMergeYAML_ListReplace(t *testing.T) {
	base := "tags:\n  - a\n  - b\n  - c\n"
	overlay := "tags:\n  - x\n"
	got, err := deepMergeYAML(base, overlay)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// list 整个被替换，不是合并
	if !strings.Contains(got, "- x") {
		t.Fatalf("应含 - x，got: %s", got)
	}
	for _, item := range []string{"- a", "- b", "- c"} {
		if strings.Contains(got, item) {
			t.Fatalf("list 应被 overlay 整个替换，不该含 %q。got: %s", item, got)
		}
	}
}

func TestDeepMergeYAML_NewKeyInOverlay(t *testing.T) {
	base := "server:\n  port: 8080\n"
	overlay := "feature_flag_x: true\n"
	got, err := deepMergeYAML(base, overlay)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "feature_flag_x: true") {
		t.Fatalf("overlay 新 key 应出现，got: %s", got)
	}
	if !strings.Contains(got, "port: 8080") {
		t.Fatalf("base 已有 key 应保留，got: %s", got)
	}
}

func TestDeepMergeYAML_TypeMismatch(t *testing.T) {
	// base 是 map、overlay 是 scalar 在同 key → overlay 整个替换 base 的 map
	base := "mysql:\n  host: a\n  port: 3306\n"
	overlay := "mysql: simple-string\n"
	got, err := deepMergeYAML(base, overlay)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "mysql: simple-string") {
		t.Fatalf("type mismatch 应让 overlay 替换 base，got: %s", got)
	}
}
