@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup.ps1"
if errorlevel 1 (
  echo Installation failed. Review the message above.
  pause
  exit /b 1
)
echo Installation complete. Open NodeLane.cmd to use the client.
pause
