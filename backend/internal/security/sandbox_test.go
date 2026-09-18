package security

import (
	"testing"

	"github.com/jjbank/skill-market/internal/model"
)

func TestIsRunnableEntry(t *testing.T) {
	yes := []string{"server.py", "pkg/main.py", "a/b/app.py", "runner.py", "index.js", "install.sh", "setup.sh", "SERVER.PY"}
	for _, p := range yes {
		if !IsRunnableEntry(p) {
			t.Fatalf("期望 %q 被识别为可执行入口", p)
		}
	}
	no := []string{"SKILL.md", "README.md", "utils.py", "server.pyc", "web.jsx", ""}
	for _, p := range no {
		if IsRunnableEntry(p) {
			t.Fatalf("期望 %q 不是可执行入口", p)
		}
	}
}

func TestHasRunnableEntry(t *testing.T) {
	files := []model.SkillFile{
		{Path: "SKILL.md", Content: []byte("---\nname: x\n---\n")},
		{Path: "requirements.txt", Content: []byte("pydantic")},
	}
	if HasRunnableEntry(files) {
		t.Fatal("无入口的包不应要求动态验证")
	}
	files = append(files, model.SkillFile{Path: "src/server.py", Content: []byte("print(1)")})
	if !HasRunnableEntry(files) {
		t.Fatal("含 server.py 的包应要求动态验证")
	}
	// 被跳过的文件 (超限/二进制) 不算入口
	skipped := []model.SkillFile{{Path: "server.py", Content: []byte("x"), Skipped: true}}
	if HasRunnableEntry(skipped) {
		t.Fatal("跳过的文件不应算作入口")
	}
}

func TestMergeFindingsRescoresVerdict(t *testing.T) {
	scan := &model.SecurityScan{Verdict: "safe", Findings: []model.SecurityFinding{
		{RuleID: "SUP-01", Category: "supply_chain", Severity: "low", Score: 4, Title: "依赖未固定"},
	}}
	rescore(scan)
	if scan.Verdict != "safe" || scan.RiskScore != 4 {
		t.Fatalf("初始结论错误: %s/%d", scan.Verdict, scan.RiskScore)
	}

	MergeFindings(scan, []model.SecurityFinding{
		{RuleID: "DYN-01", Category: "network", Severity: "critical", Title: "运行期尝试外联", Detail: "socket.connect"},
	})
	if scan.Verdict != "malicious" || scan.CriticalCount != 1 {
		t.Fatalf("动态严重项应把结论降为 malicious: %s critical=%d", scan.Verdict, scan.CriticalCount)
	}
	if !scan.Findings[0].Blocking {
		t.Fatal("critical 发现必须标记为阻断级")
	}
	if scan.Findings[0].CategoryCN == "" {
		t.Fatal("合并时未补齐分类中文名")
	}
	if scan.FindingCount != 2 {
		t.Fatalf("发现数应为 2, 实际 %d", scan.FindingCount)
	}
}

func TestSandboxEngineVersion(t *testing.T) {
	if SandboxEngine() == "" {
		t.Fatal("沙箱引擎版本不能为空")
	}
}
