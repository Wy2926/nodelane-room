@echo off
if not exist "%ProgramFiles%\NodeLaneRoom\nlroom-cli.exe" (
  echo Run Install.cmd first.
  pause
  exit /b 1
)
set "PATH=%ProgramFiles%\NodeLaneRoom;%PATH%"
powershell.exe -NoProfile -NoExit -Command "nlroom-cli --help; Write-Host ''; Write-Host 'Start here: nlroom-cli init --server https://YOUR-SERVER --name YOUR-NAME'"
