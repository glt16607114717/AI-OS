; AI-OS NSIS Custom Installer Script
; Generates WinSW XML with actual install path, then registers service

!macro customInstall
  ; Generate ai-os-agent.xml with real install paths
  FileOpen $0 "$INSTDIR\resources\winsw\ai-os-agent.xml" w
  FileWrite $0 '<service>$\r$\n'
  FileWrite $0 '  <id>AI-OS-Agent</id>$\r$\n'
  FileWrite $0 '  <name>AI-OS Agent</name>$\r$\n'
  FileWrite $0 '  <description>AI-OS Background Agent Service</description>$\r$\n'
  FileWrite $0 '  <executable>python</executable>$\r$\n'
  FileWrite $0 '  <arguments>"$INSTDIR\resources\backend\python\agent\main.py"</arguments>$\r$\n'
  FileWrite $0 '  <startmode>Automatic</startmode>$\r$\n'
  FileWrite $0 '  <onfailure action="restart" delay="10 sec"/>$\r$\n'
  FileWrite $0 '  <onfailure action="restart" delay="30 sec"/>$\r$\n'
  FileWrite $0 '  <onfailure action="restart" delay="60 sec"/>$\r$\n'
  FileWrite $0 '  <resetfailure>1 hour</resetfailure>$\r$\n'
  FileWrite $0 '  <log mode="roll-by-size">$\r$\n'
  FileWrite $0 '    <sizeThreshold>10240</sizeThreshold>$\r$\n'
  FileWrite $0 '    <keepFiles>8</keepFiles>$\r$\n'
  FileWrite $0 '  </log>$\r$\n'
  FileWrite $0 '</service>$\r$\n'
  FileClose $0

  ; Register and start the service
  ExecWait '"$INSTDIR\resources\winsw\ai-os-agent.exe" install'
  ExecWait '"$INSTDIR\resources\winsw\ai-os-agent.exe" start'
!macroend

!macro customUnInstall
  ExecWait '"$INSTDIR\resources\winsw\ai-os-agent.exe" stop'
  Sleep 2000
  ExecWait '"$INSTDIR\resources\winsw\ai-os-agent.exe" uninstall'
!macroend
