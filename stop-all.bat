@echo off
chcp 65001 >nul
title SKILL NEXUS 停止全部服务
echo 正在停止 SKILL NEXUS 全部服务...

REM 停止隧道 (ssh / cloudflared)
taskkill /IM cloudflared.exe /F >nul 2>&1
taskkill /IM ssh.exe /F >nul 2>&1

REM 停止前端 vite preview
for /f "tokens=5" %%p in ('netstat -ano ^| findstr ":4173 " ^| findstr LISTENING') do taskkill /PID %%p /F >nul 2>&1

REM 停止后端
for /f "tokens=5" %%p in ('netstat -ano ^| findstr ":8080 " ^| findstr LISTENING') do taskkill /PID %%p /F >nul 2>&1

echo 已停止 (Docker 容器保留, 如需停止: docker compose down)
pause
