package security

import (
	"path"
	"strings"

	"github.com/jjbank/skill-market/internal/model"
)

// ============================================================
// 动态沙箱 (行为验证) 侧的常量与判定
// ============================================================

const sandboxEngineVersion = "SANDBOX-ENGINE 1.0.0"

// SandboxEngine 沙箱引擎版本
func SandboxEngine() string { return sandboxEngineVersion }

// runnableEntries 可被沙箱执行的入口文件 (与沙箱服务保持一致)
var runnableEntries = map[string]bool{
	"server.py": true, "main.py": true, "app.py": true, "runner.py": true, "skill.py": true,
	"index.js": true, "install.sh": true, "setup.sh": true,
}

// IsRunnableEntry 判断路径是否为可执行入口
func IsRunnableEntry(filePath string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(filePath, "\\", "/")))
	return runnableEntries[base]
}

// HasRunnableEntry 包内是否存在可执行入口 (决定是否需要动态验证)
func HasRunnableEntry(files []model.SkillFile) bool {
	for _, f := range files {
		if f.Skipped || len(f.Content) == 0 {
			continue
		}
		if IsRunnableEntry(f.Path) {
			return true
		}
	}
	return false
}
