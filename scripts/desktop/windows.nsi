Unicode true
!include "MUI2.nsh"
!include "x64.nsh"
!include "nsDialogs.nsh"

Name "NodeLane Room"
OutFile "${OUTPUT}"
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID lzma
BrandingText "$(Branding)"
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=2052 "ProductName" "NodeLane Room"
VIAddVersionKey /LANG=2052 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "NodeLane"
VIAddVersionKey /LANG=1033 "ProductName" "NodeLane Room"
VIAddVersionKey /LANG=1033 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "LegalCopyright" "NodeLane"
ShowInstDetails hide
ShowUninstDetails hide

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
!define MUI_LANGDLL_ALWAYSSHOW
!define MUI_LANGDLL_REGISTRY_ROOT HKCU
!define MUI_LANGDLL_REGISTRY_KEY "Software\NodeLaneRoom"
!define MUI_LANGDLL_REGISTRY_VALUENAME "InstallerLanguage"
!define MUI_ABORTWARNING
!define MUI_UNABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "$(WelcomeTitle)"
!define MUI_WELCOMEPAGE_TEXT "$(WelcomeText)"
!insertmacro MUI_PAGE_WELCOME
Page custom InstallOptions InstallOptionsLeave
!define MUI_PAGE_HEADER_TEXT "$(InstallingTitle)"
!define MUI_PAGE_HEADER_SUBTEXT "$(InstallingSubtitle)"
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "$(FinishTitle)"
!define MUI_FINISHPAGE_TEXT "$(FinishText)"
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "$(OpenRoom)"
!define MUI_FINISHPAGE_RUN_FUNCTION OpenRoom
!insertmacro MUI_PAGE_FINISH

!define MUI_WELCOMEPAGE_TITLE "$(UninstallWelcomeTitle)"
!define MUI_WELCOMEPAGE_TEXT "$(UninstallWelcomeText)"
!insertmacro MUI_UNPAGE_WELCOME
UninstPage custom un.Options un.OptionsLeave
!define MUI_PAGE_HEADER_TEXT "$(UninstallingTitle)"
!define MUI_PAGE_HEADER_SUBTEXT "$(UninstallingSubtitle)"
!insertmacro MUI_UNPAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "$(UninstallFinishTitle)"
!define MUI_FINISHPAGE_TEXT "$(UninstallFinishText)"
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
!include "${__FILEDIR__}\locales\en-US.nsh"
!include "${__FILEDIR__}\locales\zh-CN.nsh"
!insertmacro MUI_RESERVEFILE_LANGDLL
SetFont /LANG=2052 "Microsoft YaHei UI" 9

Function .onInit
  SetRegView 64
  !insertmacro MUI_LANGDLL_DISPLAY
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "$(ArchitectureError)" /SD IDOK
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
  !insertmacro MUI_HEADER_TEXT "$(OptionsTitle)" "$(OptionsSubtitle)"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 22u "NodeLane Room ${VERSION} · ${ARCH}"
  Pop $0
  ${NSD_CreateLabel} 0 24u 100% 28u "$(InstallLocation)"
  Pop $0
  StrCpy $1 "$(InstallAction)"
  ${If} $CurrentVersion != ""
    StrCpy $1 "$(UpdateAction)"
  ${EndIf}
  ${NSD_CreateRadioButton} 0 62u 100% 14u "$1"
  Pop $UpdateRadio
  ${NSD_Check} $UpdateRadio
  StrCpy $RestoreRadio ""
  IfFileExists "$PROGRAMFILES64\NodeLaneRoom.previous\Uninstall.exe" 0 no_restore
    ${NSD_CreateRadioButton} 0 84u 100% 14u "$(RestoreAction)"
    Pop $RestoreRadio
    ${If} $Operation == "rollback"
      ${NSD_Uncheck} $UpdateRadio
      ${NSD_Check} $RestoreRadio
    ${EndIf}
  no_restore:
  ${NSD_CreateLabel} 0 112u 100% 62u "$(Requirements)"
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
  StrCpy $Arguments "-Quiet"
  ${If} $Operation == "rollback"
    StrCpy $Arguments "$Arguments -Rollback"
  ${EndIf}
  DetailPrint "$(InstallingDetail)"
  ; The localized wizard reports failures; PowerShell details remain in the log.
  ; Keep this UI and setup.ps1 as the player, including alternate-account UAC.
  nsExec::ExecToLog '"$PowerShell" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\engine\setup.ps1" -SourceDir "$PLUGINSDIR\payload" $Arguments'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "$(InstallError)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
SectionEnd

Function OpenRoom
  Exec '"$INSTDIR\nlroom.exe"'
FunctionEnd

Function un.onInit
  SetRegView 64
  !insertmacro MUI_UNGETLANGUAGE
  StrCpy $INSTDIR "$PROGRAMFILES64\NodeLaneRoom"
  StrCpy $PowerShell "$WINDIR\SysNative\WindowsPowerShell\v1.0\powershell.exe"
  StrCpy $PurgeState ${BST_UNCHECKED}
  StrCpy $FinishText "$(DataRetained)"
FunctionEnd

Function un.Options
  !insertmacro MUI_HEADER_TEXT "$(UninstallOptionsTitle)" "$(UninstallOptionsSubtitle)"
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 34u "$(RemovalList)"
  Pop $0
  ${NSD_CreateCheckbox} 0 52u 100% 18u "$(PurgeAction)"
  Pop $PurgeCheckbox
  ${NSD_SetState} $PurgeCheckbox $PurgeState
  ${NSD_CreateLabel} 0 84u 100% 50u "$(PurgeHelp)"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function un.OptionsLeave
  ${NSD_GetState} $PurgeCheckbox $PurgeState
  ${If} $PurgeState == ${BST_CHECKED}
    MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 "$(PurgeConfirm)" IDYES confirmed
    Abort
    confirmed:
  ${EndIf}
FunctionEnd

Section "Uninstall"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "${ENGINE}\uninstall.ps1"
  File "${ENGINE}\tap.ps1"
  SetOutPath "$PLUGINSDIR\tap"
  File /r "${ENGINE}\tap\*"
  StrCpy $Arguments "-Quiet"
  ${If} $PurgeState == ${BST_CHECKED}
    StrCpy $Arguments "$Arguments -PurgeState"
    StrCpy $FinishText "$(DataCleared)"
  ${EndIf}
  nsExec::ExecToLog '"$PowerShell" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\uninstall.ps1" $Arguments'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "$(UninstallError)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
SectionEnd
