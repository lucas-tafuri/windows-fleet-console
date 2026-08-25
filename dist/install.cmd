@echo off
title Fleet Console installer
cd /d "%~dp0"
echo.
echo ========================================
echo  Fleet Console - Windows client install
echo ========================================
echo Folder: %CD%
echo Log:    %TEMP%\fleet-console-install.log
echo Also:   %LOCALAPPDATA%\FleetConsole\install.log
echo.

if not exist "%~dp0install.ps1" (
  echo ERROR: install.ps1 was not found next to install.cmd.
  echo Copy both files from:
  echo   https://github.com/lucas-tafuri/windows-fleet-console/tree/main/dist
  echo.
  echo Press any key to close.
  pause >nul
  exit /b 1
)

where powershell >nul 2>&1
if errorlevel 1 (
  echo ERROR: PowerShell is not available on this PC.
  echo.
  echo Press any key to close.
  pause >nul
  exit /b 1
)

powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %*
set "ERR=%ERRORLEVEL%"
echo.
echo Log file: %TEMP%\fleet-console-install.log
echo Also:     %LOCALAPPDATA%\FleetConsole\install.log
if not "%ERR%"=="0" (
  echo Install finished with error code %ERR%.
)
echo.
echo Press any key to close.
pause >nul
exit /b %ERR%
