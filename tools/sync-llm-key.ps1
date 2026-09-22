# ============================================================
# 把 OpenClaw 凭据库里的模型 API Key 同步到本机用户环境变量
# 让 SkillHub 后端可直接使用同一把 Key (不打印明文, 不写入仓库)
# 用法: powershell -ExecutionPolicy Bypass -File tools\sync-llm-key.ps1
# ============================================================
param(
    [string]$Provider = "deepseek",
    [string]$EnvName  = "DEEPSEEK_API_KEY",
    [string]$SetDefaultProvider = "deepseek"
)

$ErrorActionPreference = "Stop"
$db = Join-Path $env:USERPROFILE ".openclaw\agents\main\agent\openclaw-agent.sqlite"

if (-not (Test-Path $db)) { Write-Host "[x] 未找到 OpenClaw 凭据库: $db" -ForegroundColor Red; exit 1 }

$py = Get-Command py -ErrorAction SilentlyContinue
if (-not $py) { $py = Get-Command python -ErrorAction SilentlyContinue }
if (-not $py) { Write-Host "[x] 需要 Python (py) 来读取凭据库" -ForegroundColor Red; exit 1 }

$script = @"
import sqlite3, json, sys
db = r"$db"
provider = "$Provider"
try:
    c = sqlite3.connect(db)
    row = c.execute("SELECT store_json FROM auth_profile_store WHERE store_key='primary'").fetchone()
    c.close()
    prof = json.loads(row[0])["profiles"]
    key = None
    for name, p in prof.items():
        if p.get("provider") == provider and p.get("key"):
            key = p["key"]; break
    if not key:
        print("NOKEY"); sys.exit(0)
    sys.stdout.write(key)
except Exception as e:
    print("ERR:" + str(e)); sys.exit(0)
"@
$tmp = Join-Path $env:TEMP "read_llm_key.py"
[System.IO.File]::WriteAllText($tmp, $script, (New-Object System.Text.UTF8Encoding($false)))
$key = (& $py.Source $tmp 2>$null | Out-String).Trim()
Remove-Item $tmp -Force -ErrorAction SilentlyContinue

if (-not $key -or $key -eq "NOKEY" -or $key.StartsWith("ERR:")) {
    Write-Host "[x] 未从 OpenClaw 找到 provider=$Provider 的 Key ($key)" -ForegroundColor Red
    exit 1
}

[Environment]::SetEnvironmentVariable($EnvName, $key, "User")
if ($SetDefaultProvider) { [Environment]::SetEnvironmentVariable("LLM_PROVIDER", $SetDefaultProvider, "User") }

Write-Host "[OK] 已写入用户环境变量:" -ForegroundColor Green
Write-Host "     $EnvName = $($key.Substring(0,[Math]::Min(7,$key.Length)))*** (len=$($key.Length))"
if ($SetDefaultProvider) { Write-Host "     LLM_PROVIDER = $SetDefaultProvider" }
Write-Host "   重启后端后生效: backend\skill-market-backend.exe" -ForegroundColor DarkGray
