# ============================================================
# SkillHub · 公网隧道刷新 + 网页版自动同步
# ------------------------------------------------------------
# 作用：重启 cloudflared 隧道拿到新地址 -> 写入仓库变量
#       SKILLHUB_API_BASE -> 触发 GitHub Actions 重新部署网页版
# 用法：powershell -ExecutionPolicy Bypass -File tools\refresh-tunnel.ps1
# 说明：隧道地址每次重启都会变，跑完这个脚本网页版即可重新连上后端。
# ============================================================
param(
    [string]$Repo = "TONY-384626433/skill-market-platform",
    # 隧道直指后端 8080 (API)。这样跨域预检 OPTIONS 会由后端正确返回 CORS 头；
    # 若指向前端 4173, vite 预览代理会改掉预检响应导致网页版 POST 失败。
    [int]$Port = 8080
)

$ErrorActionPreference = "Continue"
$Root = Split-Path -Parent $PSScriptRoot

function Find-Cloudflared {
    $c = Get-Command cloudflared -ErrorAction SilentlyContinue
    if ($c) { return $c.Source }
    foreach ($p in @(
        "C:\Program Files (x86)\cloudflared\cloudflared.exe",
        "C:\Program Files\cloudflared\cloudflared.exe"
    )) { if (Test-Path $p) { return $p } }
    return $null
}

Write-Host ""
Write-Host "  === SkillHub 隧道刷新 + 网页版同步 ===" -ForegroundColor Cyan

# 0. 前置检查
$cfBin = Find-Cloudflared
if (-not $cfBin) { Write-Host "  × 未找到 cloudflared，请先安装/加入 PATH" -ForegroundColor Red; exit 1 }
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { Write-Host "  × 未找到 gh CLI" -ForegroundColor Red; exit 1 }

$listen = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if (-not $listen) { Write-Host "  ! 端口 $Port 没有在监听（后端 Go API 未启动？）" -ForegroundColor Yellow }

# 1. 停掉旧隧道
Get-Process -Name cloudflared -ErrorAction SilentlyContinue | ForEach-Object {
    Write-Host "  · 停止旧隧道进程 PID $($_.Id)" -ForegroundColor Yellow
    Stop-Process -Id $_.Id -Force
}
Start-Sleep 3

# 2. 起新隧道
$logOut = "$env:TEMP\skillhub-cf.log"
$logErr = "$env:TEMP\skillhub-cf.err"
Remove-Item $logOut, $logErr -ErrorAction SilentlyContinue
Write-Host "  · 启动新隧道 -> http://localhost:$Port" -ForegroundColor Yellow
Start-Process -FilePath $cfBin -ArgumentList "tunnel", "--url", "http://localhost:$Port", "--protocol", "http2", "--no-autoupdate" `
    -WindowStyle Hidden -RedirectStandardOutput $logOut -RedirectStandardError $logErr

$url = $null
for ($i = 0; $i -lt 25; $i++) {
    Start-Sleep 2
    $all = (Get-Content $logOut -Raw -ErrorAction SilentlyContinue) + "`n" + (Get-Content $logErr -Raw -ErrorAction SilentlyContinue)
    if ($all -match "https://([a-z0-9-]+\.trycloudflare\.com)") { $url = "https://$($matches[1])"; break }
}
if (-not $url) {
    Write-Host "  × 隧道建立失败，检查网络。日志尾部：" -ForegroundColor Red
    (Get-Content $logErr -Raw -ErrorAction SilentlyContinue) -split "`n" | Select-Object -Last 8
    exit 1
}
Write-Host "  √ 隧道地址: $url" -ForegroundColor Green

# 3. 健康检查
$apiBase = "$url/api/v1"
try {
    $h = Invoke-RestMethod -Uri "$apiBase/health" -TimeoutSec 20
    Write-Host "  √ 后端健康: $($h.service) v$($h.version)" -ForegroundColor Green
} catch {
    Write-Host "  ! 隧道可达但后端健康检查失败: $($_.Exception.Message)" -ForegroundColor Yellow
}

# 4. 写入仓库变量
Write-Host "  · 写入仓库变量 SKILLHUB_API_BASE" -ForegroundColor Yellow
gh variable set SKILLHUB_API_BASE --body $apiBase --repo $Repo
if ($LASTEXITCODE -ne 0) { Write-Host "  × 变量写入失败" -ForegroundColor Red; exit 1 }
Write-Host "  √ 变量已更新" -ForegroundColor Green

# 5. 触发重新部署
Write-Host "  · 触发网页版重新部署" -ForegroundColor Yellow
gh workflow run deploy-pages.yml --repo $Repo
Start-Sleep 4
gh run list --workflow=deploy-pages.yml --repo $Repo --limit 1

Write-Host ""
Write-Host "  ────────────────────────────────────────────" -ForegroundColor Cyan
Write-Host "  √ 完成! 网页版 (约 1 分钟后生效):" -ForegroundColor Green
Write-Host "    https://tony-384626433.github.io/skill-market-platform/" -ForegroundColor White
Write-Host "  本地完整版: http://localhost:$Port" -ForegroundColor DarkGray
Write-Host "  ⚠ 电脑需保持开机，隧道才会一直通" -ForegroundColor DarkGray
Write-Host ""
