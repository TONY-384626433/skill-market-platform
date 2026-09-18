@echo off
chcp 936 >nul
title 九江银行 SkillHub · 一键启动
echo.
echo   正在启动 SkillHub（Docker 基础设施 + 技能容器 + 后端 + 前端 + 公网隧道）
echo   首次启动约需 30-60 秒，完成后会自动打开浏览器与公网地址。
echo.
powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0start-demo.ps1" -OpenBrowser
