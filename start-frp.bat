@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start-frp.ps1"
exit /b %ERRORLEVEL%
