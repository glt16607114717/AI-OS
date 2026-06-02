!macro customInit
  StrCpy $INSTDIR "C:\AI-OS"
!macroend

!macro customInstall
  IfFileExists "$TEMP\ai-os-runtime" 0 done_install
  IfFileExists "$INSTDIR\runtime" done_install 0
  Rename "$TEMP\ai-os-runtime" "$INSTDIR\runtime"

  done_install:
!macroend
