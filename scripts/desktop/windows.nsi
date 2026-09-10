Unicode true
!include "MUI2.nsh"
Name "NodeLane Room"
OutFile "${OUTPUT}"
RequestExecutionLevel user
ShowInstDetails show
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "NodeLane Room 开发测试版"
!define MUI_WELCOMEPAGE_TEXT "安装玩家界面、网络后台与 Wintun。此测试包未进行 NodeLane 代码签名。安装期间需要管理员授权，日常联机使用当前玩家账户。仅支持全新安装。"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "打开 NodeLane Room"
!define MUI_FINISHPAGE_RUN_FUNCTION OpenRoom
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_LANGUAGE "SimpChinese"

Section "NodeLane Room"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\payload"
  File /r "${PAYLOAD}\*"
  ; This process remains the player. setup.ps1 captures its SID before UAC,
  ; even when elevation uses a different administrator account.
  nsExec::ExecToLog '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\payload\setup.ps1"'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "安装未完成。请查看安装详情。已有身份不会被自动清除。"
    Abort
  ${EndIf}
SectionEnd

Function OpenRoom
  SetRegView 64
  ReadRegStr $0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom" "InstallLocation"
  ${If} $0 != ""
    Exec '"$0\nlroom.exe"'
  ${EndIf}
FunctionEnd
