@echo off
rem SkillHub Java 桌面客户端 — 启动脚本
setlocal
cd /d "%~dp0"

set "JAVA=java"
if defined JAVA_HOME if exist "%JAVA_HOME%\bin\java.exe" set "JAVA=%JAVA_HOME%\bin\java.exe"

if not exist "dist\SkillHubDesktop.jar" (
  echo [*] 未找到 dist\SkillHubDesktop.jar，正在编译...
  powershell -ExecutionPolicy Bypass -File "%~dp0build.ps1" || goto :err
)

echo [*] 启动 SkillHub 桌面客户端 (Java)...
"%JAVA%" -Dfile.encoding=UTF-8 -jar "dist\SkillHubDesktop.jar"
goto :eof

:err
echo [x] 构建失败，请检查 JDK 环境。
pause
