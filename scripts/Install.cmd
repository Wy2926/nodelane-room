@echo off
"%~dp0nlroom-update.exe" setup --source "%~dp0."
if errorlevel 1 (
  echo Installation failed. Review the message above.
  pause
  exit /b 1
)
echo Installation complete. Open NodeLaneRoom.cmd to use the client CLI.
pause
