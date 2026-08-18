# ============================================================
# SkillHub — 一键启动 (比赛演示模式)
# 启动: Docker 基础设施 + 技能容器 → Go 后端 → 前端 → 公网隧道
# 用法: 双击 start-demo.bat 或运行 powershell -File start-demo.ps1
# ============================================================

$ErrorActionPreference = "Continue"
$ROOT = Split-Path -Parent $MyInvocation.MyCommand.Path

function Test-Port($port) {
    return [bool](Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)
}

Write-Host ""
Write-Host "  ╔══════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "  ║      SkillHub · 一键启动 (演示模式)       ║" -ForegroundColor Cyan
Write-Host "  ╚══════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# ============ 1. Docker 基础设施 + 技能服务 ============
Write-Host "[1/5] Docker 基础设施 + 技能容器 ..." -ForegroundColor Yellow
docker info *> $null
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ✗ Docker 未运行, 请先启动 Docker Desktop" -ForegroundColor Red
} else {
    Push-Location "$ROOT\docker"
    docker compose up -d --build 2>&1 | Out-Null
    Pop-Location
    $runner = docker ps --filter "name=skillhub-runner" --format "{{.Names}}" 2>$null
    if ($runner) { Write-Host "  ✓ 技能容器运行中: $runner" -ForegroundColor Green }
    else { Write-Host "  ⚠ 技能容器未就绪, 等待 5s 重试..." -ForegroundColor Yellow; Start-Sleep 5 }
}

# ============ 2. Go 后端 (8080) ============
Write-Host "[2/5] Go 后端 API ..." -ForegroundColor Yellow
if (Test-Port 8080) {
    Write-Host "  ✓ 后端已在运行 (8080)" -ForegroundColor Green
} else {
    # 已登录 GitHub CLI 时，将 Token 仅传给后端子进程，用于提升公开 Skill 搜索额度。
    $previousGitHubToken = $env:GITHUB_TOKEN
    if (-not $env:GITHUB_TOKEN) {
        $githubCLI = Get-Command gh -ErrorAction SilentlyContinue
        if ($githubCLI) {
            $detectedGitHubToken = (& gh auth token 2>$null)
            if ($LASTEXITCODE -eq 0 -and $detectedGitHubToken) {
                $env:GITHUB_TOKEN = $detectedGitHubToken.Trim()
                Write-Host "  ✓ 已连接 GitHub 公开技能索引" -ForegroundColor Green
            }
        }
    }
    Start-Process -FilePath "$ROOT\backend\skill-market-backend.exe" -WorkingDirectory "$ROOT\backend" -WindowStyle Hidden
    if ($previousGitHubToken) { $env:GITHUB_TOKEN = $previousGitHubToken }
    elseif (Test-Path Env:GITHUB_TOKEN) { Remove-Item Env:GITHUB_TOKEN }
    Start-Sleep 3
    if (Test-Port 8080) { Write-Host "  ✓ 后端启动成功 (8080)" -ForegroundColor Green }
    else { Write-Host "  ✗ 后端启动失败! 检查 backend\skill-market-backend.exe" -ForegroundColor Red }
}

# ============ 3. 前端预览 (4173) ============
Write-Host "[3/5] 前端页面 ..." -ForegroundColor Yellow
if (Test-Port 4173) {
    Write-Host "  ✓ 前端已在运行 (4173)" -ForegroundColor Green
} else {
    Push-Location "$ROOT\frontend"
    Start-Process -FilePath "npx.cmd" -ArgumentList "vite","preview","--port","4173","--host" -WorkingDirectory "$ROOT\frontend" -WindowStyle Hidden
    Pop-Location
    Start-Sleep 4
    if (Test-Port 4173) { Write-Host "  ✓ 前端启动成功 (4173)" -ForegroundColor Green }
    else { Write-Host "  ✗ 前端启动失败! 检查 frontend\node_modules" -ForegroundColor Red }
}

# ============ 4. 健康检查 ============
Write-Host "[4/5] 健康检查 ..." -ForegroundColor Yellow
Start-Sleep 2
try {
    $h = Invoke-RestMethod -Uri "http://localhost:8080/api/health" -TimeoutSec 5
    Write-Host "  ✓ 后端健康: $($h.service) v$($h.version)" -ForegroundColor Green
} catch { Write-Host "  ✗ 后端不可达" -ForegroundColor Red }
try {
    $s = Invoke-RestMethod -Uri "http://localhost:4173/api/v1/skills?page_size=1" -TimeoutSec 8
    Write-Host "  ✓ 技能市场: $($s.total) 个技能" -ForegroundColor Green
} catch { Write-Host "  ✗ 技能数据不可达" -ForegroundColor Red }

# ============ 5. 公网隧道 ============
Write-Host "[5/5] 公网隧道 ..." -ForegroundColor Yellow
$tunnelUrl = $null

# 5a. 先试 localhost.run (免注册, 国内可用)
$lhrLog = "$env:TEMP\skillhub-lhr.log"
Remove-Item $lhrLog -ErrorAction SilentlyContinue
Start-Process -FilePath "ssh" -ArgumentList "-o","StrictHostKeyChecking=no","-o","ServerAliveInterval=30","-N","-R","80:localhost:4173","nokey@localhost.run" -WindowStyle Hidden -RedirectStandardOutput $lhrLog -RedirectStandardError "$env:TEMP\skillhub-lhr.err"
for ($i = 0; $i -lt 15; $i++) {
    Start-Sleep 2
    $log = Get-Content $lhrLog -Raw -ErrorAction SilentlyContinue
    $logErr = Get-Content "$env:TEMP\skillhub-lhr.err" -Raw -ErrorAction SilentlyContinue
    $all = $log + "`n" + $logErr
    if ($all -match "https://([a-z0-9]+\.lhr\.life)") {
        $tunnelUrl = "https://$($matches[1])"
        Write-Host "  ✓ localhost.run 隧道: $tunnelUrl" -ForegroundColor Green
        break
    }
}

# 5b. localhost.run 不可用时回退 Cloudflare
if (-not $tunnelUrl) {
    Write-Host "  ⚠ localhost.run 不可用, 尝试 Cloudflare ..." -ForegroundColor Yellow
    $cf = Get-Command cloudflared -ErrorAction SilentlyContinue
    if (-not $cf) { $cfPath = "C:\Program Files (x86)\cloudflared\cloudflared.exe"; if (Test-Path $cfPath) { $cf = $cfPath } }
    if ($cf) {
        $logFile = "$env:TEMP\skillhub-cf.log"
        # $cf 可能是命令对象或字符串路径, 统一取二进制路径
        $cfBin = if ($cf -is [string]) { $cf } else { $cf.Source }
        $proc = Start-Process -FilePath $cfBin -ArgumentList "tunnel","--url","http://localhost:4173","--protocol","http2","--no-autoupdate" -WindowStyle Hidden -RedirectStandardOutput $logFile -RedirectStandardError "$env:TEMP\skillhub-cf.err"
        for ($i = 0; $i -lt 12; $i++) {
            Start-Sleep 2
            $log = Get-Content $logFile -Raw -ErrorAction SilentlyContinue
            $logErr = Get-Content "$env:TEMP\skillhub-cf.err" -Raw -ErrorAction SilentlyContinue
            $all = $log + "`n" + $logErr
            if ($all -match "https://([a-z0-9-]+\.trycloudflare\.com)") {
                $tunnelUrl = "https://$($matches[1])"
                Write-Host "  ✓ Cloudflare 隧道: $tunnelUrl" -ForegroundColor Green
                break
            }
        }
        if (-not $tunnelUrl) { Write-Host "  ✗ 隧道建立失败 (网络问题?), 本地地址仍可用" -ForegroundColor Red }
    } else {
        Write-Host "  ✗ 未找到 cloudflared, 仅本地访问" -ForegroundColor Red
    }
}

# ============ 结果 ============
Write-Host ""
Write-Host "  ────────────────────────────────────────────" -ForegroundColor Cyan
Write-Host "   ✅ 启动完成! 访问地址:" -ForegroundColor Green
Write-Host ""
Write-Host "   本地完整版:  http://localhost:4173" -ForegroundColor White
if ($tunnelUrl) { Write-Host "   公网完整版:  $tunnelUrl" -ForegroundColor White }
Write-Host ""
Write-Host "   登录账号:    zhangsan / lisi / admin (密码: demo)" -ForegroundColor DarkGray
Write-Host "  ────────────────────────────────────────────" -ForegroundColor Cyan
Write-Host ""
Write-Host "  提示: 公网隧道 URL 每次启动会变化, 电脑需保持开机" -ForegroundColor DarkGray
Write-Host "  停止全部: 运行 stop-all.bat" -ForegroundColor DarkGray
Write-Host ""
pause
