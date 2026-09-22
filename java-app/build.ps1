# ============================================================
# SkillHub Java Desktop Client - build (compile + jar), no Maven
# Usage: powershell -ExecutionPolicy Bypass -File build.ps1
# (ASCII-only messages on purpose: avoid PowerShell 5.1 encoding issues)
# ============================================================
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Src = Join-Path $Root "src"
$Out = Join-Path $Root "out"
$Dist = Join-Path $Root "dist"

# Locate JDK: JAVA_HOME -> PATH -> common install dirs
$jdk = $env:JAVA_HOME
if (-not $jdk -or -not (Test-Path "$jdk\bin\javac.exe")) {
    $c = Get-Command javac -ErrorAction SilentlyContinue
    if ($c) { $jdk = Split-Path -Parent (Split-Path -Parent $c.Source) }
}
if (-not $jdk -or -not (Test-Path "$jdk\bin\javac.exe")) {
    $roots = @("C:\Program Files\Microsoft", "C:\Program Files\Eclipse Adoptium", "C:\Program Files\Java", "C:\Program Files\Amazon Corretto", "C:\Program Files\Zulu")
    $jdk = $null
    foreach ($r in $roots) {
        if (-not (Test-Path $r)) { continue }
        $hit = Get-ChildItem $r -Directory -ErrorAction SilentlyContinue |
               Where-Object { Test-Path (Join-Path $_.FullName "bin\javac.exe") } |
               Sort-Object Name -Descending | Select-Object -First 1
        if ($hit) { $jdk = $hit.FullName; break }
    }
}
if (-not $jdk -or -not (Test-Path "$jdk\bin\javac.exe")) {
    Write-Host "[x] JDK not found. Install a JDK and set JAVA_HOME." -ForegroundColor Red
    exit 1
}
Write-Host "[*] JDK: $jdk" -ForegroundColor Cyan

Remove-Item $Out -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $Out, $Dist | Out-Null

$javaFiles = Get-ChildItem -Path $Src -Recurse -Filter *.java | ForEach-Object { $_.FullName }
if (-not $javaFiles) { Write-Host "[x] no .java files under src" -ForegroundColor Red; exit 1 }

Write-Host "[*] compiling $($javaFiles.Count) source file(s) ..." -ForegroundColor Yellow
& "$jdk\bin\javac.exe" -encoding UTF-8 -d $Out $javaFiles
if ($LASTEXITCODE -ne 0) { Write-Host "[x] compile failed" -ForegroundColor Red; exit 1 }

Write-Host "[*] packaging jar ..." -ForegroundColor Yellow
$jar = Join-Path $Dist "SkillHubDesktop.jar"
& "$jdk\bin\jar.exe" --create --file $jar --main-class com.skillhub.desktop.App -C $Out .
if ($LASTEXITCODE -ne 0) { Write-Host "[x] jar failed" -ForegroundColor Red; exit 1 }

$size = [math]::Round((Get-Item $jar).Length / 1KB, 1)
Write-Host ""
Write-Host "[OK] built: $jar ($size KB)" -ForegroundColor Green
Write-Host "     run:  java -jar `"$jar`"   (or double-click run.bat)" -ForegroundColor DarkGray
