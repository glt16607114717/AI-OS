!macro customInit
  ReadRegStr $INSTDIR HKLM "Software\AI-OS" "InstallDir"
  StrCmp $INSTDIR "" 0 has_path
    StrCpy $INSTDIR "C:\AI-OS"
    Goto done_init
  has_path:
    ExecWait 'net stop AI-OS-Agent' $0
    Sleep 1000
    ExecWait 'taskkill /F /IM pythonw.exe' $0
    ExecWait 'taskkill /F /IM AI-OS.exe' $0
    Sleep 1000
  done_init:
!macroend

!macro customInstall
  WriteRegStr HKLM "Software\AI-OS" "InstallDir" "$INSTDIR"
!macroend
