@echo off
title Fleet Console install log
set "LOG1=%TEMP%\fleet-console-install.log"
set "LOG2=%LOCALAPPDATA%\FleetConsole\install.log"
echo Opening install log...
echo   %LOG1%
echo   %LOG2%
if exist "%LOG2%" (
  notepad "%LOG2%"
) else if exist "%LOG1%" (
  notepad "%LOG1%"
) else (
  echo No log file found yet. Run install.cmd first.
  pause
)
