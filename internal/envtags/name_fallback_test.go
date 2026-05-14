package envtags

import "testing"

func TestEnvTagOrNameFallback(t *testing.T) {
	if got := EnvTagOrNameFallback("staging", "app-PROD-1"); got != "staging" {
		t.Fatalf("tag set: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "xx-PROD-yy"); got != "生产" {
		t.Fatalf("prod: got %q", got)
	}
	if got := EnvTagOrNameFallback("  ", "TEST-db"); got != "测试" {
		t.Fatalf("test: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "my-DEV-box"); got != "开发" {
		t.Fatalf("dev: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "svc-UAT-01"); got != "验收" {
		t.Fatalf("uat: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "DEV-PROD-mix"); got != "开发" {
		t.Fatalf("earliest dev before prod: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "plain"); got != "" {
		t.Fatalf("no match: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "DFT工时系统-PROD-应用-0002"); got != "生产" {
		t.Fatalf("segmented prod: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "LC_测试_应用"); got != "测试" {
		t.Fatalf("underscore + chinese 测试: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "svc-生产-01"); got != "生产" {
		t.Fatalf("chinese 生产: got %q", got)
	}
	if got := EnvTagOrNameFallback("", "app-验收"); got != "验收" {
		t.Fatalf("chinese 验收: got %q", got)
	}
}

func TestNodeTagOrNameFallback(t *testing.T) {
	if got := NodeTagOrNameFallback("自定义", "x-LC"); got != "自定义" {
		t.Fatalf("tag set: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "host-DFT-01"); got != "德非图" {
		t.Fatalf("dft: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "LC-web"); got != "宇海" {
		t.Fatalf("lc: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "LC-DFT"); got != "宇海" {
		t.Fatalf("earliest LC before DFT: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "DFT-LC"); got != "德非图" {
		t.Fatalf("earliest DFT before LC: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "DFT工时系统-PROD-应用-0002"); got != "德非图" {
		t.Fatalf("segmented DFT prefix: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "LC_测试_应用"); got != "宇海" {
		t.Fatalf("LC first segment: got %q", got)
	}
	if got := NodeTagOrNameFallback("ALL", "ignored"); got != "全部" {
		t.Fatalf("tag ALL: got %q", got)
	}
	if got := NodeTagOrNameFallback("all", "ignored"); got != "全部" {
		t.Fatalf("tag all: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "app_ALL_prod"); got != "全部" {
		t.Fatalf("segment ALL: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "DFT-ALL"); got != "德非图" {
		t.Fatalf("DFT before ALL segment: got %q", got)
	}
	if got := NodeTagOrNameFallback("", "ALL-LC"); got != "全部" {
		t.Fatalf("ALL before LC segment: got %q", got)
	}
}

func TestResourceNameForTagFallback(t *testing.T) {
	pairs := [][2]string{{"Name", "from-name-tag"}, {"env", "x"}}
	if got := ResourceNameForTagFallback("", "id-1", pairs); got != "from-name-tag" {
		t.Fatalf("name tag: got %q", got)
	}
	if got := ResourceNameForTagFallback("direct", "id-1", pairs); got != "direct" {
		t.Fatalf("instance name wins: got %q", got)
	}
	if got := ResourceNameForTagFallback("", "", nil); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}
