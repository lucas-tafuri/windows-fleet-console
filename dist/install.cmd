@echo off
setlocal DisableDelayedExpansion
title PrettyDamnFleet installer
cd /d "%~dp0"
echo.
echo ========================================
echo  PrettyDamnFleet - Windows client install
echo ========================================
echo Install or reconnect this PC to the manager.
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

rem Relaunch this same menu elevated; already-elevated launches skip UAC.
powershell.exe -NoLogo -NoProfile -Command "if (([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { exit 0 } else { exit 1 }"
if not errorlevel 1 goto elevated
echo Requesting administrator access...
set "FLEET_INSTALL_CMD=%~f0"
set FLEET_INSTALL_ARGS=%*
powershell.exe -NoLogo -NoProfile -Command "try { $arguments = '/d /s /c '+[char]34+[char]34+$env:FLEET_INSTALL_CMD+[char]34+' '+$env:FLEET_INSTALL_ARGS+[char]34; $process = Start-Process -FilePath $env:ComSpec -ArgumentList $arguments -WorkingDirectory (Split-Path -LiteralPath $env:FLEET_INSTALL_CMD) -Verb RunAs -Wait -PassThru; exit $process.ExitCode } catch { Write-Host 'Administrator access was cancelled or could not be started.'; exit 1 }"
set "ERR=%ERRORLEVEL%"
if not "%ERR%"=="0" pause
exit /b %ERR%

:elevated
set "PAIRING_OPTION="
if not "%~1"=="" goto run_installer
echo  1. Install / update using the saved manager
echo  2. Forget old token and rediscover manager
echo.
choice /C 12 /N /M "Choose 1 or 2: "
if errorlevel 3 exit /b 1
if errorlevel 2 set "PAIRING_OPTION=-ResetPairing"
if not errorlevel 1 exit /b 1
echo.

:run_installer
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" %PAIRING_OPTION% %*
set "ERR=%ERRORLEVEL%"
echo.
echo Log file: %TEMP%\fleet-console-install.log
echo Also:     %ProgramData%\FleetConsole\install.log
if not "%ERR%"=="0" (
  echo Install finished with error code %ERR%.
)
echo.
echo Press any key to close.
pause >nul
exit /b %ERR%
