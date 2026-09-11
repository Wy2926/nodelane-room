Unicode true
!include "MUI2.nsh"
!include "x64.nsh"
!include "nsDialogs.nsh"

Name "NodeLane Room"
OutFile "${OUTPUT}"
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID lzma
BrandingText "NodeLane · 和朋友在同一房间"
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=2052 "ProductName" "NodeLane Room"
VIAddVersionKey /LANG=2052 "FileDescription" "NodeLane Room 安装与维护"
VIAddVersionKey /LANG=2052 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "NodeLane"
ShowInstDetails nevershow
ShowUninstDetails nevershow

Var CurrentVersion
Var Operation
Var RestoreRadio
Var UpdateRadio
Var PurgeCheckbox
Var PurgeState
Var FinishText
Var Arguments
Var PowerShell

!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom"
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "${ART}\header.bmp"
!define MUI_WELCOMEFINISHPAGE_BITMAP "${ART}\welcome.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "${ART}\welcome.bmp"
!define MUI_ABORTWARNING
!define MUI_UNABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "欢迎使用$\r$\nNodeLane Room"
!define MUI_WELCOMEPAGE_TEXT "和朋友在同一房间。$\r$\n$\r$\n此向导将安装游戏联机客户端与网络后台，或维护已有安装。$\r$\n$\r$\n版本 ${VERSION} · ${ARCH}$\r$\n开发测试版 · 尚未进行代码签名$\r$\n$\r$\n请从日常游戏使用的 Windows 账户运行。点击安装后，Windows 将请求管理员授权。"
!insertmacro MUI_PAGE_WELCOME
Page custom InstallOptions InstallOptionsLeave
!define MUI_PAGE_HEADER_TEXT "正在配置 NodeLane Room"
!define MUI_PAGE_HEADER_SUBTEXT "请稍候，正在校验程序并配置网络后台。"
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "NodeLane Room 已准备好"
!define MUI_FINISHPAGE_TEXT "安装已完成。打开客户端，和朋友开始联机。$\r$\n$\r$\n可从 Windows 设置中的“已安装的应用”卸载。再次运行完整安装包可更新或恢复上一版程序。"
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "打开 NodeLane Room"
!define MUI_FINISHPAGE_RUN_FUNCTION OpenRoom
!insertmacro MUI_PAGE_FINISH

!define MUI_WELCOMEPAGE_TITLE "卸载$\r$\nNodeLane Room"
!define MUI_WELCOMEPAGE_TEXT "此向导将移除 NodeLane Room 客户端与网络后台。$\r$\n$\r$\n卸载会结束当前联机，关闭本机隧道。$\r$\n$\r$\n默认保留设备身份与用户绑定，重新安装后可继续使用。下一步可选择同时清除本机数据。"
!insertmacro MUI_UNPAGE_WELCOME
UninstPage custom un.Options un.OptionsLeave
!define MUI_PAGE_HEADER_TEXT "正在卸载 NodeLane Room"
!define MUI_PAGE_HEADER_SUBTEXT "正在停止联机并移除程序，请稍候。"
!insertmacro MUI_UNPAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "NodeLane Room 已卸载"
!define MUI_FINISHPAGE_TEXT "客户端与网络后台已移除。$\r$\n$\r$\n$FinishText$\r$\n$\r$\n感谢使用 NodeLane Room。"
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "SimpChinese"
SetFont /LANG=2052 "Microsoft YaHei UI" 9

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "请选择与系统架构对应的 64 位安装包。" /SD IDOK
    SetErrorLevel 1
    Quit
  ${EndIf}
  SetRegView 64
  StrCpy $INSTDIR "$PROGRAMFILES64\NodeLaneRoom"
  StrCpy $PowerShell "$WINDIR\SysNative\WindowsPowerShell\v1.0\powershell.exe"
  StrCpy $Operation "install"
  ReadRegStr $CurrentVersion HKLM "${UNINSTALL_KEY}" "DisplayVersion"
FunctionEnd

Function InstallOptions
  !insertmacro MUI_HEADER_TEXT "安装与维护" "确认版本和位置，选择此次操作。"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 22u "NodeLane Room ${VERSION} · ${ARCH}"
  Pop $0
  ${NSD_CreateLabel} 0 24u 100% 28u "安装位置（由系统保护）：$\r$\n$INSTDIR"
  Pop $0
  StrCpy $1 "安装 NodeLane Room"
  ${If} $CurrentVersion != ""
    StrCpy $1 "安装 / 更新至 ${VERSION}（当前 $CurrentVersion）"
  ${EndIf}
  ${NSD_CreateRadioButton} 0 62u 100% 14u "$1"
  Pop $UpdateRadio
  ${NSD_Check} $UpdateRadio
  StrCpy $RestoreRadio ""
  IfFileExists "$PROGRAMFILES64\NodeLaneRoom.previous\Uninstall.exe" 0 no_restore
    ${NSD_CreateRadioButton} 0 84u 100% 14u "恢复上一次安装的程序（保留当前设备身份）"
    Pop $RestoreRadio
    ${If} $Operation == "rollback"
      ${NSD_Uncheck} $UpdateRadio
      ${NSD_Check} $RestoreRadio
    ${EndIf}
  no_restore:
  ${NSD_CreateLabel} 0 112u 100% 42u "包含客户端和网络后台；LAN 需要预先准备专用 TAP 网卡。缺少 WebView2 时联网安装微软运行时。$\r$\n更新会短暂中断联机；失败时自动恢复原程序。"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function InstallOptionsLeave
  StrCpy $Operation "install"
  ${If} $RestoreRadio != ""
    ${NSD_GetState} $RestoreRadio $0
    ${If} $0 == ${BST_CHECKED}
      StrCpy $Operation "rollback"
    ${EndIf}
  ${EndIf}
FunctionEnd

Section "NodeLane Room"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\engine"
  File /r "${ENGINE}\*"
  SetOutPath "$PLUGINSDIR\payload"
  File /r "${PAYLOAD}\*"
  WriteUninstaller "$PLUGINSDIR\payload\Uninstall.exe"
  StrCpy $Arguments ""
  ${If} $Operation == "rollback"
    StrCpy $Arguments "-Rollback"
  ${EndIf}
  IfSilent 0 +2
    StrCpy $Arguments "$Arguments -Quiet"
  DetailPrint "正在验证安装包、申请管理员授权并配置后台…"
  ; Keep this UI and setup.ps1 as the player, including alternate-account UAC.
  nsExec::ExecToLog '"$PowerShell" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\engine\setup.ps1" -SourceDir "$PLUGINSDIR\payload" $Arguments'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "安装未完成。管理员授权可能被取消，或程序校验 / 后台启动失败。请查看错误提示后重试。设备身份不会自动清除。" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
SectionEnd

Function OpenRoom
  Exec '"$INSTDIR\nlroom.exe"'
FunctionEnd

Function un.onInit
  SetRegView 64
  StrCpy $INSTDIR "$PROGRAMFILES64\NodeLaneRoom"
  StrCpy $PowerShell "$WINDIR\SysNative\WindowsPowerShell\v1.0\powershell.exe"
  StrCpy $PurgeState ${BST_UNCHECKED}
  StrCpy $FinishText "已保留设备身份与用户绑定，重新安装后可继续使用。"
FunctionEnd

Function un.Options
  !insertmacro MUI_HEADER_TEXT "确认卸载" "选择是否保留本机数据，然后点击卸载。"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 34u "将移除：客户端、网络后台、开始菜单入口及旧版备份。$\r$\n$INSTDIR"
  Pop $0
  ${NSD_CreateCheckbox} 0 52u 100% 18u "同时清除设备身份与用户绑定"
  Pop $PurgeCheckbox
  ${NSD_SetState} $PurgeCheckbox $PurgeState
  ${NSD_CreateLabel} 0 84u 100% 50u "默认不勾选，方便以后重新安装。$\r$\n勾选后，本机身份数据无法恢复；重新安装需重新初始化。$\r$\n这不会删除服务器上的房间，也不会卸载共享的 WebView2。"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function un.OptionsLeave
  ${NSD_GetState} $PurgeCheckbox $PurgeState
  ${If} $PurgeState == ${BST_CHECKED}
    MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 "确定清除本机设备身份与用户绑定？此操作无法撤销。" IDYES confirmed
    Abort
    confirmed:
  ${EndIf}
FunctionEnd

Section "Uninstall"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "${ENGINE}\uninstall.ps1"
  StrCpy $Arguments ""
  ${If} $PurgeState == ${BST_CHECKED}
    StrCpy $Arguments "-PurgeState"
    StrCpy $FinishText "本机设备身份与用户绑定已清除。重新安装后需重新初始化。"
  ${EndIf}
  IfSilent 0 +2
    StrCpy $Arguments "$Arguments -Quiet"
  nsExec::ExecToLog '"$PowerShell" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\uninstall.ps1" $Arguments'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "卸载未完成。请检查管理员授权或错误提示后重试。" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
SectionEnd
