@echo off
if not exist "%ProgramFiles%\NodeLaneRoom\nodelane.exe" (
  echo Run Install.cmd first.
  pause
  exit /b 1
)
set "PATH=%ProgramFiles%\NodeLaneRoom;%PATH%"
powershell.exe -NoProfile -NoExit -Command "nodelane --help; Write-Host ''; Write-Host 'Start here: nodelane init --server https://YOUR-SERVER --name YOUR-NAME'"
