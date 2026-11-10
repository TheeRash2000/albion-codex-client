; Codex Client installer (Albion Codex, https://albioncodex.com)
;
; Based on albiondata-client.nsi, Copyright (c) 2017 The Albion Data Project.
; See the LICENSE file in the root folder (MIT).
;
; A per-user install: no administrator rights, nothing written outside the
; user's own profile and HKEY_CURRENT_USER. The client replaces its own exe when
; it self-updates, so it has to live somewhere the user can write.
;
; Build (from the repo root, after building codex-client.exe):
;   makensis -DPACKAGE_VERSION=0.1.19 -DTOP_SRCDIR=..\.. pkg\nsis\codex-client.nsi
; PACKAGE_VERSION must be numeric (major.minor.patch).

!include "MUI2.nsh"
!include "LogicLib.nsh"

!ifndef PACKAGE_VERSION
  !define PACKAGE_VERSION "0.0.0"
!endif
!ifndef TOP_SRCDIR
  !define TOP_SRCDIR "..\.."
!endif
!ifndef OUTFILE
  !define OUTFILE "${TOP_SRCDIR}\codex-client-setup.exe"
!endif

!define PACKAGE_NAME "Codex Client"
!define PACKAGE_EXE "codex-client.exe"
!define PACKAGE_PUBLISHER "Albion Codex"
!define PACKAGE_URL "https://albioncodex.com"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\CodexClient"
; The login entry the client registers for itself (client/autostart_win.go).
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define RUN_VALUE "CodexClient"

Unicode true
CRCCheck on
SetCompressor /SOLID lzma

Name "${PACKAGE_NAME}"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\${PACKAGE_NAME}"
InstallDirRegKey HKCU "Software\${PACKAGE_NAME}" ""

VIProductVersion "${PACKAGE_VERSION}.0"
VIAddVersionKey "CompanyName" "${PACKAGE_PUBLISHER}"
VIAddVersionKey "FileDescription" "${PACKAGE_NAME} Installer"
VIAddVersionKey "FileVersion" "${PACKAGE_VERSION}"
VIAddVersionKey "ProductName" "${PACKAGE_NAME}"
VIAddVersionKey "ProductVersion" "${PACKAGE_VERSION}"
VIAddVersionKey "LegalCopyright" "MIT licensed. Portions Copyright (c) 2017 The Albion Data Project"

!define MUI_ICON "${TOP_SRCDIR}\icon\codex-client.ico"
!define MUI_UNICON "${TOP_SRCDIR}\icon\codex-client.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PACKAGE_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start ${PACKAGE_NAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${TOP_SRCDIR}\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

; The exe cannot be replaced or removed while the client is running. Ask the
; user to quit it rather than ending the process for them.
!macro WaitForClientToClose
  ${Do}
    ${IfNot} ${FileExists} "$INSTDIR\${PACKAGE_EXE}"
      ${ExitDo}
    ${EndIf}
    ClearErrors
    FileOpen $0 "$INSTDIR\${PACKAGE_EXE}" a
    ${IfNot} ${Errors}
      FileClose $0
      ${ExitDo}
    ${EndIf}
    ; A silent run cannot ask, so it stops instead of waiting forever.
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION \
      "${PACKAGE_NAME} is running. Quit it from its icon in the system tray, then choose Retry." \
      /SD IDCANCEL IDRETRY +2
    Abort
  ${Loop}
!macroend

Section "Install"
  !insertmacro WaitForClientToClose

  SetOutPath "$INSTDIR"
  File "${TOP_SRCDIR}\${PACKAGE_EXE}"
  File /oname=LICENSE.txt "${TOP_SRCDIR}\LICENSE"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateShortCut "$SMPROGRAMS\${PACKAGE_NAME}.lnk" "$INSTDIR\${PACKAGE_EXE}"

  WriteRegStr HKCU "Software\${PACKAGE_NAME}" "" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${PACKAGE_NAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${PACKAGE_VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${PACKAGE_EXE},0"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "${PACKAGE_PUBLISHER}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "URLInfoAbout" "${PACKAGE_URL}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1

  ; Npcap is what lets the client read game traffic. It is not bundled (its
  ; licence does not allow redistribution); point the user at the official
  ; download when it is missing. The installer is a 32-bit program, so this
  ; reads the 32-bit registry view, which is where Npcap registers itself.
  ReadRegStr $0 HKLM "SOFTWARE\Npcap" ""
  ${If} $0 == ""
    MessageBox MB_YESNO|MB_ICONINFORMATION \
      "${PACKAGE_NAME} needs Npcap to read game traffic, and Npcap does not appear to be installed.$\n$\nOpen the Npcap download page now?" \
      /SD IDNO IDNO +2
    ExecShell "open" "https://npcap.com/#download"
  ${EndIf}
SectionEnd

Section "Uninstall"
  !insertmacro WaitForClientToClose

  Delete "$INSTDIR\${PACKAGE_EXE}"
  ; Left behind by the client's self-update.
  Delete "$INSTDIR\.${PACKAGE_EXE}.old"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\${PACKAGE_NAME}.lnk"

  DeleteRegValue HKCU "${RUN_KEY}" "${RUN_VALUE}"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  DeleteRegKey HKCU "Software\${PACKAGE_NAME}"
SectionEnd
