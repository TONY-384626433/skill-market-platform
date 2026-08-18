@echo off
chcp 65001 >nul
title SkillHub 一键启动
powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0start-demo.ps1"
