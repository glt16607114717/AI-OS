!macro customInit
  ; 读取旧的安装目录
  ReadRegStr $INSTDIR HKLM "Software\AI-OS" "InstallDir"
  ReadRegStr $INSTDIR HKCU "Software\AI-OS" "InstallDir"

  ; 如果读取不到，设置默认值
  StrCmp $INSTDIR "" set_default
  StrCmp $INSTDIR "C:\AI-OS" 0 done_init
  set_default:
    ; 检查旧的安装路径是否存在
    IfFileExists "C:\AI-OS\AI-OS.exe" old_install new_install

  old_install:
    StrCpy $INSTDIR "C:\AI-OS"
    Goto done_init

  new_install:
    StrCpy $INSTDIR "$LOCALAPPDATA\AI-OS"

  done_init:
!macroend

!macro customInstall
  ; ── 写清理脚本到固定数据目录（提权后一定能访问）──
  CreateDirectory "C:\ProgramData\AI-OS"
  FileOpen $0 "C:\ProgramData\AI-OS\cleanup.ps1" w
  FileWrite $0 `schtasks /End /TN "AI-OS-Watchdog" 2>&1 | Out-Null$\r$\n`
  FileWrite $0 `schtasks /Delete /TN "AI-OS-Watchdog" /F 2>&1 | Out-Null$\r$\n`
  FileWrite $0 `schtasks /End /TN "AI-OS-Agent" 2>&1 | Out-Null$\r$\n`
  FileWrite $0 `schtasks /Delete /TN "AI-OS-Agent" /F 2>&1 | Out-Null$\r$\n`
  FileWrite $0 `Start-Sleep -Seconds 2$\r$\n`
  FileWrite $0 `Stop-Process -Name pythonw -Force -ErrorAction SilentlyContinue$\r$\n`
  FileWrite $0 `Stop-Process -Name python -Force -ErrorAction SilentlyContinue$\r$\n`
  FileWrite $0 `Stop-Process -Name AI-OS -Force -ErrorAction SilentlyContinue$\r$\n`
  FileWrite $0 `Start-Sleep -Seconds 1$\r$\n`
  FileWrite $0 `if (Test-Path "C:\ProgramData\AI-OS") { icacls "C:\ProgramData\AI-OS" /grant "BUILTIN\Users:(OI)(CI)F" /T /Q }$\r$\n`
  FileClose $0

  ; ── 一次提权执行所有操作（先删任务 → 再杀进程 → 修权限）──
  nsExec::ExecToLog 'powershell.exe -NoProfile -WindowStyle Hidden -Command "Start-Process powershell -Verb RunAs -Wait -ArgumentList ''-NoProfile'',''-ExecutionPolicy'',''Bypass'',''-File'',''C:\ProgramData\AI-OS\cleanup.ps1''"'
  Pop $0
  Pop $1

  ; 删除临时脚本
  Delete "C:\ProgramData\AI-OS\cleanup.ps1"

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
