!macro customInit
  ReadRegStr $INSTDIR HKLM "Software\AI-OS" "InstallDir"
  StrCmp $INSTDIR "" 0 has_path
    StrCpy $INSTDIR "C:\AI-OS"
    Goto done_init
  has_path:
    ; 覆盖安装：停服务、杀进程（在文件解压前执行）
    ; 先停 watchdog 防止拉起
    nsExec::ExecToStack 'schtasks /End /TN "AI-OS-Watchdog"'
    Pop $0
    Pop $1
    nsExec::ExecToStack 'schtasks /Delete /TN "AI-OS-Watchdog" /F'
    Pop $0
    Pop $1
    nsExec::ExecToStack 'schtasks /End /TN "AI-OS-Agent"'
    Pop $0
    Pop $1
    nsExec::ExecToStack 'schtasks /Delete /TN "AI-OS-Agent" /F'
    Pop $0
    Pop $1
    Sleep 1000
    nsExec::ExecToStack 'taskkill /F /IM pythonw.exe'
    Pop $0
    Pop $1
    nsExec::ExecToStack 'taskkill /F /IM AI-OS.exe'
    Pop $0
    Pop $1
    Sleep 1000
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
