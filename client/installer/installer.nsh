!macro customInit
  ReadRegStr $INSTDIR HKLM "Software\AI-OS" "InstallDir"
  StrCmp $INSTDIR "" 0 has_path
    StrCpy $INSTDIR "C:\AI-OS"
    Goto done_init
  has_path:
    ; Overwrite install: stop service and kill processes
    nsExec::Exec '"$INSTDIR\resources\winsw\ai-os-agent.exe" stop'
    nsExec::Exec '"$INSTDIR\resources\winsw\ai-os-agent.exe" uninstall'
    Sleep 500
    nsExec::Exec 'taskkill /F /IM AI-OS.exe'
    Sleep 500
  done_init:
!macroend

!macro customInstall
  ; Ensure path ends with \AI-OS
  StrCpy $0 $INSTDIR "" -6
  StrCmp $0 "\AI-OS" write_reg
    StrCpy $INSTDIR "$INSTDIR\AI-OS"
  write_reg:
    WriteRegStr HKLM "Software\AI-OS" "InstallDir" "$INSTDIR"
!macroend
