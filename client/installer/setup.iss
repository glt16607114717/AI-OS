#define MyAppName "AI-OS"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "NNDRobot"
#define MyAppURL "https://nndrobot.com"

[Setup]
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
DefaultDirName={autopf}\AI-OS
DefaultGroupName=AI-OS
UninstallDisplayName=AI-OS
UninstallDisplayIcon={app}\AI-OS.exe
OutputDir=..\release-v3\installer
OutputBaseFilename=AI-OS-Setup-{#MyAppVersion}
Compression=lzma2/normal
SolidCompression=no
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
DisableWelcomePage=no
DisableDirPage=no
DisableProgramGroupPage=yes
CloseApplications=no
AllowCancelDuringInstall=False

[Messages]
StatusExtractFiles=正在解压文件...
StatusCreateIcons=正在创建快捷方式...
StatusCreateShortcuts=正在创建快捷方式...
StatusRegisterFiles=正在注册文件...
StatusSavingUninstall=正在保存卸载信息...
StatusRollback=正在回滚更改...
StatusClosingApplications=正在关闭应用程序...
StatusRestartingApplications=正在重启应用程序...
StatusCreateDirs=正在创建目录...

[Files]
Source: "..\release-v3\win-unpacked\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\AI-OS"; Filename: "{app}\AI-OS.exe"
Name: "{group}\卸载 AI-OS"; Filename: "{uninstallexe}"
Name: "{autodesktop}\AI-OS"; Filename: "{app}\AI-OS.exe"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加选项:"

[Run]
Filename: "{app}\AI-OS.exe"; Description: "运行 AI-OS"; Flags: nowait postinstall skipifsilent unchecked

[UninstallDelete]
Type: filesandordirs; Name: "{app}\runtime"

[Code]
type
  TMsg = record
    hwnd: HWND;
    message: Cardinal;
    wParam: Longint;
    lParam: Longint;
    time: Cardinal;
    pt: TPoint;
  end;

function PeekMessageMsg(var lpMsg: TMsg; hWnd: HWND; wMsgFilterMin: UINT; wMsgFilterMax: UINT; wRemoveMsg: UINT): BOOL; external 'PeekMessageW@user32.dll stdcall';
function TranslateMessageMsg(var lpMsg: TMsg): BOOL; external 'TranslateMessage@user32.dll stdcall';
function DispatchMessageMsg(var lpMsg: TMsg): Longint; external 'DispatchMessageW@user32.dll stdcall';

const
  PM_REMOVE = 1;

procedure ProcessMessages;
var
  Msg: TMsg;
begin
  while PeekMessageMsg(Msg, 0, 0, 0, PM_REMOVE) do
  begin
    TranslateMessageMsg(Msg);
    DispatchMessageMsg(Msg);
  end;
end;

var
  ProgressLabel: TLabel;
  DetailLabel: TLabel;
  ProgressFile: String;

procedure InitializeWizard;
begin
  WizardForm.ProgressGauge.Min := 0;
  WizardForm.ProgressGauge.Max := 100;

  ProgressLabel := TLabel.Create(WizardForm);
  ProgressLabel.Parent := WizardForm;
  ProgressLabel.Left := WizardForm.ProgressGauge.Left;
  ProgressLabel.Top := WizardForm.ProgressGauge.Top + WizardForm.ProgressGauge.Height + 16;
  ProgressLabel.Width := 400;
  ProgressLabel.Height := 20;
  ProgressLabel.Font.Size := 9;
  ProgressLabel.AutoSize := False;
  ProgressLabel.Caption := '';

  DetailLabel := TLabel.Create(WizardForm);
  DetailLabel.Parent := WizardForm;
  DetailLabel.Left := WizardForm.ProgressGauge.Left;
  DetailLabel.Top := ProgressLabel.Top + 24;
  DetailLabel.Width := 400;
  DetailLabel.Height := 120;
  DetailLabel.Font.Size := 8;
  DetailLabel.Font.Color := clGray;
  DetailLabel.AutoSize := False;
  DetailLabel.WordWrap := True;
  DetailLabel.Caption := '';

  WizardForm.BackButton.Caption := '< 上一步(&B)';
  WizardForm.NextButton.Caption := '下一步(&N) >';
  WizardForm.CancelButton.Caption := '取消';
end;

procedure CurPageChanged(CurPageID: Integer);
begin
  WizardForm.BackButton.Caption := '< 上一步(&B)';
  WizardForm.CancelButton.Caption := '取消';

  case CurPageID of
    wpWelcome:
    begin
      WizardForm.WelcomeLabel1.Caption := '欢迎使用 AI-OS 安装向导';
      WizardForm.WelcomeLabel2.Caption := '安装程序将在您的计算机上安装 AI-OS。' + #13#10 + #13#10 + '建议在继续之前关闭所有其他应用程序。' + #13#10 + #13#10 + '单击"下一步"继续，或单击"取消"退出安装向导。';
      WizardForm.NextButton.Caption := '下一步(&N) >';
    end;
    wpSelectDir:
    begin
      WizardForm.PageNameLabel.Caption := '选择安装位置';
      WizardForm.PageDescriptionLabel.Caption := '选择 AI-OS 的安装文件夹';
      WizardForm.SelectDirLabel.Caption := '安装程序将把 AI-OS 安装到以下文件夹。';
      WizardForm.SelectDirBrowseLabel.Caption := '单击"下一步"继续。如果您想选择其他文件夹，请单击"浏览"。';
      WizardForm.DiskSpaceLabel.Caption := '所需磁盘空间';
      WizardForm.NextButton.Caption := '下一步(&N) >';
    end;
    wpPreparing:
    begin
      WizardForm.PageNameLabel.Caption := '正在准备安装';
      WizardForm.PageDescriptionLabel.Caption := '安装程序正在准备安装 AI-OS';
      WizardForm.PreparingLabel.Caption := '安装程序正在准备在您的计算机上安装 AI-OS。';
      WizardForm.NextButton.Caption := '下一步(&N) >';
    end;
    wpReady:
    begin
      WizardForm.PageNameLabel.Caption := '准备安装';
      WizardForm.PageDescriptionLabel.Caption := '安装程序已准备好开始安装';
      WizardForm.ReadyLabel.Caption := '安装程序已准备好开始在您的计算机上安装 AI-OS。' + #13#10 + #13#10 + '注意：安装过程需要下载 Python 运行时环境（约 50MB），请确保网络连接正常。' + #13#10 + #13#10 + '单击"安装"开始安装。';
      WizardForm.NextButton.Caption := '安装(&I)';
    end;
    wpInstalling:
    begin
      WizardForm.PageNameLabel.Caption := '正在安装';
      WizardForm.PageDescriptionLabel.Caption := '请稍候，AI-OS 正在安装中...';
    end;
    wpFinished:
    begin
      WizardForm.PageNameLabel.Caption := '安装完成';
      WizardForm.PageDescriptionLabel.Caption := 'AI-OS 已成功安装';
      WizardForm.NextButton.Caption := '完成(&F)';
    end;
  end;
end;

procedure SetProgress(Step, TotalSteps: Integer; const Msg, Detail: String);
var
  Pct: Integer;
begin
  Pct := Round((Step / TotalSteps) * 100);
  WizardForm.ProgressGauge.Position := Pct;
  ProgressLabel.Caption := Msg;
  DetailLabel.Caption := Detail;
  WizardForm.Repaint;
end;

function IsServiceRunning(const ServiceName: String): Boolean;
var
  ResultCode: Integer;
begin
  Result := False;
  if Exec('sc.exe', 'query ' + ServiceName + '', '', SW_HIDE, ewWaitUntilTerminated, ResultCode) then
  begin
    Result := (ResultCode = 0);
  end;
end;

function SendShutdownSignal(const AppDir: String): Boolean;
var
  PythonExe, AgentScript, Cmd: String;
  ResultCode: Integer;
begin
  Result := False;
  PythonExe := AppDir + '\runtime\python\python.exe';
  AgentScript := AppDir + '\resources\backend\python\agent\main.py';

  if FileExists(PythonExe) and FileExists(AgentScript) and IsServiceRunning('AI-OS-Agent') then
  begin
    Cmd := Format('"%s" -c "import urllib.request; urllib.request.urlopen(''http://127.0.0.1:18731/shutdown'', timeout=5).read()"', [PythonExe]);
    Exec('cmd.exe', '/c ' + Cmd, '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Result := (ResultCode = 0);
  end;
end;

function GetTickCount64: Cardinal; external 'GetTickCount@kernel32.dll stdcall';

function WaitForServiceStop(const ServiceName: String; MaxSeconds: Integer): Boolean;
var
  StartTime, Elapsed: Cardinal;
begin
  StartTime := GetTickCount64;
  while (GetTickCount64 - StartTime < MaxSeconds * 1000) do
  begin
    if not IsServiceRunning(ServiceName) then
    begin
      Result := True;
      Exit;
    end;
    Sleep(500);
  end;
  Result := False;
end;

function RunPowerShellScript(const AppDir, ScriptName: String; const MainMsg: String): Boolean;
var
  ScriptPath, ProgressFilePath, DoneFilePath, Cmd: String;
  ResultCode: Integer;
  LastLineCount, WaitCount: Integer;
  FileText: TArrayOfString;
  I: Integer;
begin
  Result := False;
  ScriptPath := AppDir + '\resources\backend\python\' + ScriptName;
  ProgressFilePath := AppDir + '\.progress.log';
  DoneFilePath := AppDir + '\.progress.done';

  if FileExists(ProgressFilePath) then
    DeleteFile(ProgressFilePath);
  if FileExists(DoneFilePath) then
    DeleteFile(DoneFilePath);

  LastLineCount := 0;
  ProgressLabel.Caption := MainMsg;
  DetailLabel.Caption := '';
  WizardForm.Repaint;

  Cmd := Format('-ExecutionPolicy Bypass -NoProfile -File "%s" "%s" -ProgressFile "%s"', [ScriptPath, AppDir, ProgressFilePath]);

  if not Exec('powershell.exe', Cmd, '', SW_HIDE, ewNoWait, ResultCode) then
  begin
    DetailLabel.Caption := 'Failed to start PowerShell';
    WizardForm.Repaint;
    Result := False;
    Exit;
  end;

  WaitCount := 0;
  while not FileExists(DoneFilePath) do
  begin
    if FileExists(ProgressFilePath) then
    begin
      LoadStringsFromFile(ProgressFilePath, FileText);
      if GetArrayLength(FileText) > LastLineCount then
      begin
        for I := LastLineCount to GetArrayLength(FileText) - 1 do
        begin
          DetailLabel.Caption := FileText[I];
        end;
        LastLineCount := GetArrayLength(FileText);
      end;
    end;
    WizardForm.Repaint;
    ProcessMessages;
    Sleep(200);
    WaitCount := WaitCount + 1;
    if WaitCount > 3000 then
    begin
      DetailLabel.Caption := 'Operation timed out (10 min)';
      WizardForm.Repaint;
      Result := False;
      Exit;
    end;
  end;

  if FileExists(DoneFilePath) then
  begin
    LoadStringsFromFile(DoneFilePath, FileText);
    if (GetArrayLength(FileText) > 0) and (FileText[0] = '0') then
      Result := True
    else
      Result := False;
    DeleteFile(DoneFilePath);
  end;

  if FileExists(ProgressFilePath) then
  begin
    LoadStringsFromFile(ProgressFilePath, FileText);
    if GetArrayLength(FileText) > 0 then
      DetailLabel.Caption := FileText[GetArrayLength(FileText) - 1];
    DeleteFile(ProgressFilePath);
  end;

  WizardForm.Repaint;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  AppDir, WinswExe, XmlPath, XmlContent: String;
  ResultCode: Integer;
  PythonExe: String;
begin
  if CurStep = ssInstall then
  begin
    AppDir := ExpandConstant('{app}');
    WinswExe := AppDir + '\resources\winsw\ai-os-agent.exe';

    if FileExists(WinswExe) and IsServiceRunning('AI-OS-Agent') then
    begin
      SetProgress(1, 6, '步骤 1/6：正在停止旧服务...', '发送优雅关闭信号，等待服务停止');
      SendShutdownSignal(AppDir);
      if not WaitForServiceStop('AI-OS-Agent', 30) then
      begin
        Exec(WinswExe, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
        Sleep(2000);
      end;
    end
    else
    begin
      SetProgress(1, 6, '步骤 1/6：正在准备安装...', '解压应用程序文件');
    end;

    SetProgress(2, 6, '步骤 2/6：正在解压应用程序文件...', '正在将程序文件复制到目标文件夹');
  end;

  if CurStep = ssPostInstall then
  begin
    AppDir := ExpandConstant('{app}');
    WinswExe := AppDir + '\resources\winsw\ai-os-agent.exe';
    PythonExe := AppDir + '\runtime\python\python.exe';

    if not RunPowerShellScript(AppDir, 'download_python.ps1', '步骤 3/6：正在配置 Python 运行环境...') then
    begin
      RaiseException('Python 环境配置失败，安装已中断。');
    end;

    SetProgress(4, 6, '步骤 4/6：正在安装 Python 依赖...', '');

    if not RunPowerShellScript(AppDir, 'install_deps.ps1', '步骤 4/6：正在安装 Python 依赖...') then
    begin
      RaiseException('Python 依赖安装失败，安装已中断。');
    end;

    SetProgress(5, 6, '步骤 5/6：正在注册后台服务...', '生成服务配置文件并注册系统服务');

    XmlPath := AppDir + '\resources\winsw\ai-os-agent.xml';
    XmlContent :=
      '<service>' + #13#10 +
      '  <id>AI-OS-Agent</id>' + #13#10 +
      '  <name>AI-OS Agent</name>' + #13#10 +
      '  <description>AI-OS Background Agent Service</description>' + #13#10 +
      '  <executable>' + AppDir + '\runtime\python\pythonw.exe</executable>' + #13#10 +
      '  <arguments>"' + AppDir + '\resources\backend\python\agent\main.py"</arguments>' + #13#10 +
      '  <startmode>Automatic</startmode>' + #13#10 +
      '  <onfailure action="restart" delay="10 sec"/>' + #13#10 +
      '  <onfailure action="restart" delay="30 sec"/>' + #13#10 +
      '  <onfailure action="restart" delay="60 sec"/>' + #13#10 +
      '  <resetfailure>1 hour</resetfailure>' + #13#10 +
      '  <log mode="roll-by-size">' + #13#10 +
      '    <sizeThreshold>10240</sizeThreshold>' + #13#10 +
      '    <keepFiles>8</keepFiles>' + #13#10 +
      '  </log>' + #13#10 +
      '</service>';

    SaveStringToFile(XmlPath, XmlContent, False);

    if FileExists(WinswExe) then
    begin
      SetProgress(5, 6, '步骤 5/6：正在注册后台服务...', '停止旧服务 → 更新配置 → 重新注册 → 启动');

      if FileExists(XmlPath) then
      begin
        Exec(WinswExe, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
        Sleep(1000);
        Exec(WinswExe, 'uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
        Sleep(1000);
        Exec(WinswExe, 'install', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
        Sleep(1000);
        Exec(WinswExe, 'start', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
      end;
    end;

    SetProgress(6, 6, '步骤 6/6：安装完成！', 'AI-OS 已成功安装并运行，单击"完成"关闭安装向导。');
    Sleep(2000);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  AppDir, WinswExe: String;
  ResultCode: Integer;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    AppDir := ExpandConstant('{app}');
    WinswExe := AppDir + '\resources\winsw\ai-os-agent.exe';

    if FileExists(WinswExe) then
    begin
      SendShutdownSignal(AppDir);
      Sleep(5000);

      if IsServiceRunning('AI-OS-Agent') then
      begin
        Exec(WinswExe, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
        Sleep(3000);
      end;

      Exec(WinswExe, 'uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    end;
  end;
end;