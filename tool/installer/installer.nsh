!macro customInit
  ; 读取旧的安装目录
  ReadRegStr $INSTDIR HKLM "Software\AI-OS" "InstallDir"
  ReadRegStr $INSTDIR HKCU "Software\AI-OS" "InstallDir"

  ; 如果读取不到，设置默认值
  StrCmp $INSTDIR "" set_default
  StrCmp $INSTDIR "C:\AI-OS" 0 has_path
  set_default:
    ; 检查旧的安装路径是否存在
    IfFileExists "C:\AI-OS\AI-OS.exe" old_install new_install

  old_install:
    StrCpy $INSTDIR "C:\AI-OS"
    Goto has_path

  new_install:
    StrCpy $INSTDIR "$LOCALAPPDATA\AI-OS"
    Goto done_init

  ; ── 覆盖安装：停服务、杀进程（在文件解压前执行）──
  has_path:
    ; 创建日志目录
    CreateDirectory "C:\ProgramData\AI-OS"

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

    ; 杀进程
    nsExec::ExecToStack 'taskkill /F /IM pythonw.exe'
    Pop $0
    Pop $1
    nsExec::ExecToStack 'taskkill /F /IM python.exe'
    Pop $0
    Pop $1
    nsExec::ExecToStack 'taskkill /F /IM AI-OS.exe'
    Pop $0
    Pop $1
    Sleep 1000

  done_init:
!macroend

!macro customInstall
  ; ── 设置目录权限 ──
  CreateDirectory "C:\ProgramData\AI-OS"
  nsExec::ExecToLog 'icacls "C:\ProgramData\AI-OS" /grant "BUILTIN\Users:(OI)(CI)F" /T /Q'

  ; ── 写入注册表（记录安装路径）──
  UserInfo::GetAccountType
  Pop $0
  StrCmp $0 "Admin" 0 write_reg_user

  StrCpy $0 $INSTDIR "" -6
  StrCmp $0 "\AI-OS" write_reg_admin
    StrCpy $INSTDIR "$INSTDIR\AI-OS"
  write_reg_admin:
  WriteRegStr HKLM "Software\AI-OS" "InstallDir" "$INSTDIR"
  Goto done_install

  write_reg_user:
  StrCpy $0 $INSTDIR "" -6
  StrCmp $0 "\AI-OS" write_reg_user_final
    StrCpy $INSTDIR "$INSTDIR\AI-OS"
  write_reg_user_final:
  WriteRegStr HKCU "Software\AI-OS" "InstallDir" "$INSTDIR"

  done_install:
!macroend
