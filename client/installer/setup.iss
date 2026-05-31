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
OutputDir=..\release-v2\installer
OutputBaseFilename=AI-OS-Setup-{#MyAppVersion}
Compression=lzma2/ultra64
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
DisableWelcomePage=no
DisableDirPage=no
DisableProgramGroupPage=yes
CloseApplications=no

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
Source: "..\release-v2\win-unpacked\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\AI-OS"; Filename: "{app}\AI-OS.exe"
Name: "{group}\卸载 AI-OS"; Filename: "{uninstallexe}"
Name: "{autodesktop}\AI-OS"; Filename: "{app}\AI-OS.exe"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加选项:"

[Run]
Filename: "{app}\AI-OS.exe"; Description: "运行 AI-OS"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}\runtime"

[Code]
var
  ProgressLabel: TLabel;
  DetailLabel: TLabel;

procedure InitializeWizard;
begin
  WizardForm.ProgressGauge.Min := 0;
  WizardForm.ProgressGauge.Max := 100;

  ProgressLabel := TLabel.Create(WizardForm.InstallingPage);
  ProgressLabel.Parent := WizardForm.InstallingPage;
  ProgressLabel.Left := WizardForm.ProgressGauge.Left;
  ProgressLabel.Top := WizardForm.ProgressGauge.Top + WizardForm.ProgressGauge.Height + 12;
  ProgressLabel.Width := WizardForm.InstallingPage.Width;
  ProgressLabel.Height := 20;
  ProgressLabel.Font.Size := 9;
  ProgressLabel.Caption := '';

  DetailLabel := TLabel.Create(WizardForm.InstallingPage);
  DetailLabel.Parent := WizardForm.InstallingPage;
  DetailLabel.Left := WizardForm.ProgressGauge.Left;
  DetailLabel.Top := ProgressLabel.Top + 24;
  DetailLabel.Width := WizardForm.InstallingPage.Width;
  DetailLabel.Height := 40;
  DetailLabel.Font.Size := 8;
  DetailLabel.Font.Color := clGray;
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
      WizardForm.ReadyLabel.Caption := '安装程序已准备好开始在您的计算机上安装 AI-OS。' + #13#10 + #13#10 + '单击"安装"开始安装。';
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

procedure CurStepChanged(CurStep: TSetupStep);
var
  AppDir: String;
  WinswExe: String;
  XmlPath: String;
  XmlContent: String;
  SetupScript: String;
  PythonZip: String;
  ResultCode: Integer;
begin
  if CurStep = ssInstall then
  begin
    WinswExe := ExpandConstant('{app}') + '\resources\winsw\ai-os-agent.exe';
    if FileExists(WinswExe) then
    begin
      Exec(WinswExe, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
      Sleep(2000);
    end;
    SetProgress(1, 6, '步骤 1/6：正在安装应用程序文件...', '正在将程序文件复制到目标文件夹');
  end;

  if CurStep = ssPostInstall then
  begin
    AppDir := ExpandConstant('{app}');
    WinswExe := AppDir + '\resources\winsw\ai-os-agent.exe';
    SetupScript := AppDir + '\resources\backend\python\setup_env.ps1';
    PythonZip := AppDir + '\resources\python-runtime.zip';

    if FileExists(PythonZip) then
    begin
      SetProgress(2, 6, '步骤 2/6：正在检查并配置 Python 运行环境...', '检测版本，如有更新则重新解压');
      if FileExists(SetupScript) then
        Exec('powershell.exe',
          '-ExecutionPolicy Bypass -NoProfile -File "' + SetupScript + '" "' + AppDir + '"',
          '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    end else
    begin
      SetProgress(2, 6, '步骤 2/6：未找到 Python 运行环境包', '将在首次启动时配置');
    end;

    SetProgress(3, 6, '步骤 3/6：正在注册后台服务...', '更新服务配置并重启');

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
      SetProgress(4, 6, '步骤 4/6：正在重启后台服务...', '停止旧服务 → 更新配置 → 重新注册 → 启动');
      Exec(WinswExe, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
      Sleep(2000);
      Exec(WinswExe, 'uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
      Sleep(1000);
      Exec(WinswExe, 'install', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
      Sleep(2000);
      Exec(WinswExe, 'start', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    end;

    SetProgress(5, 6, '步骤 5/6：正在验证安装...', '检查服务状态');

    SetProgress(6, 6, '步骤 6/6：安装完成！', 'AI-OS 已准备就绪，单击"完成"关闭安装向导。');
    Sleep(1500);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  AppDir: String;
  WinswExe: String;
  ResultCode: Integer;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    AppDir := ExpandConstant('{app}');
    WinswExe := AppDir + '\resources\winsw\ai-os-agent.exe';

    if FileExists(WinswExe) then
    begin
      Exec(WinswExe, 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
      Sleep(3000);
      Exec(WinswExe, 'uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    end;
  end;
end;
