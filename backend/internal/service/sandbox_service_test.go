package service

import (
	"context"
	"testing"

	"github.com/jjbank/skill-market/internal/model"
)

func TestSandboxClientDefaults(t *testing.T) {
	t.Setenv("SKILLHUB_DYNAMIC_SANDBOX", "")
	t.Setenv("SKILLHUB_SANDBOX_URL", "")
	t.Setenv("SKILLHUB_SANDBOX_TIMEOUT", "")
	c := NewSandboxClient()
	if !c.Enabled() {
		t.Fatal("默认应启用动态沙箱")
	}
	if c.URL() != "http://localhost:8090" {
		t.Fatalf("默认沙箱地址错误: %s", c.URL())
	}
}

func TestSandboxClientDisableAndOverride(t *testing.T) {
	t.Setenv("SKILLHUB_DYNAMIC_SANDBOX", "0")
	t.Setenv("SKILLHUB_SANDBOX_URL", "http://sandbox:8090/")
	t.Setenv("SKILLHUB_SANDBOX_TIMEOUT", "90s")
	c := NewSandboxClient()
	if c.Enabled() {
		t.Fatal("SKILLHUB_DYNAMIC_SANDBOX=0 时应禁用")
	}
	if c.URL() != "http://sandbox:8090" {
		t.Fatalf("地址应去掉尾部斜杠: %s", c.URL())
	}
	if c.timeout.Seconds() != 90 {
		t.Fatalf("超时解析错误: %v", c.timeout)
	}
}

func TestSandboxVerifyDisabled(t *testing.T) {
	t.Setenv("SKILLHUB_DYNAMIC_SANDBOX", "false")
	c := NewSandboxClient()
	rep := c.Verify(context.Background(), "k", []model.SkillFile{{Path: "server.py", Content: []byte("print(1)")}})
	if rep == nil || rep.Status != "skipped" || rep.Notice == "" {
		t.Fatalf("禁用时应返回 skipped 报告: %+v", rep)
	}
}

func TestSandboxVerifyNoEntry(t *testing.T) {
	t.Setenv("SKILLHUB_DYNAMIC_SANDBOX", "1")
	t.Setenv("SKILLHUB_SANDBOX_URL", "http://127.0.0.1:1") // 不可达, 但无入口时应提前跳过
	c := NewSandboxClient()
	rep := c.Verify(context.Background(), "k", []model.SkillFile{{Path: "SKILL.md", Content: []byte("---\nname: x\n---\n")}})
	if rep == nil || rep.Status != "skipped" {
		t.Fatalf("无入口应跳过而不是请求沙箱: %+v", rep)
	}
}

func TestSandboxVerifyUnreachable(t *testing.T) {
	t.Setenv("SKILLHUB_DYNAMIC_SANDBOX", "1")
	t.Setenv("SKILLHUB_SANDBOX_URL", "http://127.0.0.1:1")
	c := NewSandboxClient()
	rep := c.Verify(context.Background(), "k", []model.SkillFile{{Path: "server.py", Content: []byte("print(1)")}})
	if rep == nil || rep.Status != "unreachable" {
		t.Fatalf("沙箱不可达应返回 unreachable: %+v", rep)
	}
	if rep.Notice == "" {
		t.Fatal("不可达时必须给出提示, 不能静默跳过")
	}
}

func TestSandboxEngineMetaShape(t *testing.T) {
	t.Setenv("SKILLHUB_DYNAMIC_SANDBOX", "0")
	meta := SandboxEngineMeta()
	if meta["enabled"] != false {
		t.Fatalf("禁用时 enabled 应为 false: %+v", meta)
	}
	if _, ok := meta["url"]; !ok {
		t.Fatal("引擎元信息应包含 url")
	}
}
