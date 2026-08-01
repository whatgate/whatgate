@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-WhatGate.ps1"
if errorlevel 1 (
  echo.
  echo Installation failed. Please send the error above to your administrator.
  pause
)
