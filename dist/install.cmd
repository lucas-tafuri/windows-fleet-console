@echo off
title Fleet Console installer
cd /d "%~dp0"
echo.
echo ========================================
echo  Fleet Console — Windows client install
echo ========================================
echo Folder: %CD%
echo.

if not exist "%~dp0install.ps1" (
  echo ERROR: install.ps1 was not found next to install.cmd.
  echo Copy both files from the dist folder, or download:
  echo   https://github.com/lucas-tafuri/windows-fleet-console/tree/main/dist
  echo.
  pause
  exit /b 1
)

where powershell >nul 2>&1
if errorlevel 1 (
  echo ERROR: PowerShell is not available on this PC.
  echo.
  pause
  exit /b 1
)

powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
set "ERR=%ERRORLEVEL%"
echo.
if not "%ERR%"=="0" (
  echo Install finished with error code %ERR%.
) else (
  echo Done. Leave this window open until you have read the log above.
)
echo.
pause
exit /b %ERR%
