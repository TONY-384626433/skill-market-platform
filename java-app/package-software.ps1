# ============================================================
# SkillHub Java Desktop Client - package as Windows software
#   Step 1: app-image (portable, bundled runtime, no install)
#   Step 2: .exe installer (needs WiX Toolset 3.x: candle.exe + light.exe in PATH)
# Usage: powershell -ExecutionPolicy Bypass -File package-software.ps1
# (ASCII-only messages: avoid PowerShell 5.1 encoding issues)
# ============================================================
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Icon = Join-Path (Split-Path -Parent $Root) "tools\assets\skillhub.ico"

# Locate JDK
$jdk = $env:JAVA_HOME
if (-not $jdk -or -not (Test-Path "$jdk\bin\jpackage.exe")) {
    $roots = @("C:\Program Files\Microsoft", "C:\Program Files\Eclipse Adoptium", "C:\Program Files\Java")
    foreach ($r in $roots) {
        if (-not (Test-Path $r)) { continue }
        $hit = Get-ChildItem $r -Directory -ErrorAction SilentlyContinue |
               Where-Object { Test-Path (Join-Path $_.FullName "bin\jpackage.exe") } |
               Sort-Object Name -Descending | Select-Object -First 1
        if ($hit) { $jdk = $hit.FullName; break }
    }
}
if (-not $jdk) { Write-Host "[x] JDK (jpackage) not found." -ForegroundColor Red; exit 1 }
Write-Host "[*] JDK: $jdk" -ForegroundColor Cyan

# Ensure jar is built
if (-not (Test-Path "$Root\dist\SkillHubDesktop.jar")) {
    Write-Host "[*] jar missing, running build.ps1 ..." -ForegroundColor Yellow
    & powershell -ExecutionPolicy Bypass -File (Join-Path $Root "build.ps1")
}

$common = @(
    "--name", "SkillHub",
    "--input", "$Root\dist",
    "--main-jar", "SkillHubDesktop.jar",
    "--main-class", "com.skillhub.desktop.App",
    "--vendor", "SkillHub",
    "--app-version", "1.1.0",
    "--description", "SkillHub Desktop Client (Java)",
    "--add-modules", "java.base,java.desktop,java.net.http,java.logging",
    "--java-options", "-Dfile.encoding=UTF-8"
)
if (Test-Path $Icon) { $common += @("--icon", $Icon) }

# Step 1: app-image (portable)
Write-Host "[*] building app-image (portable) ..." -ForegroundColor Yellow
Remove-Item "$Root\release" -Recurse -Force -ErrorAction SilentlyContinue
& "$jdk\bin\jpackage.exe" --type app-image @common --dest "$Root\release"
if ($LASTEXITCODE -ne 0) { Write-Host "[x] app-image failed" -ForegroundColor Red; exit 1 }
Write-Host "[OK] portable: $Root\release\SkillHub\SkillHub.exe" -ForegroundColor Green

$size = (Get-ChildItem "$Root\release\SkillHub" -Recurse -File | Measure-Object Length -Sum).Sum
Write-Host "     size: $([math]::Round($size/1MB,1)) MB" -ForegroundColor DarkGray

# Step 2: .exe installer (optional, needs WiX)
$candle = Get-Command candle.exe -ErrorAction SilentlyContinue
if (-not $candle) {
    Write-Host "[!] WiX (candle.exe) not in PATH - skipping .exe installer." -ForegroundColor Yellow
    Write-Host "    Install WiX 3.x or put wix314 binaries on PATH, then rerun." -ForegroundColor DarkGray
    exit 0
}

Write-Host "[*] building .exe installer ..." -ForegroundColor Yellow
Remove-Item "$Root\release-installer" -Recurse -Force -ErrorAction SilentlyContinue
& "$jdk\bin\jpackage.exe" --type exe @common --dest "$Root\release-installer" --win-menu --win-shortcut --win-dir-chooser --win-per-user-install
if ($LASTEXITCODE -ne 0) { Write-Host "[x] installer failed" -ForegroundColor Red; exit 1 }
Write-Host "[OK] installer: $Root\release-installer\SkillHub-1.1.0.exe" -ForegroundColor Green
