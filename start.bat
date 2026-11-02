@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start-retail.ps1"
exit /b %ERRORLEVEL%
