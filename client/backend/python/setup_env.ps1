param(
    [Parameter(Position=0)][string]$InstallDir,
    [Parameter(Position=1)][string]$Action = 'all'
)

$ErrorActionPreference = 'Stop'

if (-not $InstallDir) {
    Write-Host '[错误] 用法: setup_env.ps1 <安装目录> [status|python|all]'
    exit 1
}

$RuntimeDir = Join-Path $InstallDir 'runtime'
$PythonDir = Join-Path $RuntimeDir 'python'
$StatusDir = Join-Path $RuntimeDir 'status'
$LogFile = Join-Path $RuntimeDir 'setup_env.log'
$pythonExe = Join-Path $PythonDir 'python.exe'
$zipPath = Join-Path $InstallDir 'resources\python-runtime.zip'
$versionFile = Join-Path $PythonDir '.version'

New-Item -ItemType Directory -Path $RuntimeDir -Force | Out-Null
New-Item -ItemType Directory -Path $PythonDir -Force | Out-Null
New-Item -ItemType Directory -Path $StatusDir -Force | Out-Null

function Write-Log {
    param([string]$Message)
    $ts = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
    $line = "[$ts] $Message"
    Write-Host $line
    Add-Content -Path $LogFile -Value $line -Encoding UTF8
}

function Write-Status {
    param([string]$Name, [string]$Value)
    $filePath = Join-Path $StatusDir "$Name.txt"
    Set-Content -Path $filePath -Value $Value -Encoding UTF8
}

function Read-Status {
    param([string]$Name)
    $filePath = Join-Path $StatusDir "$Name.txt"
    if (Test-Path $filePath) { return (Get-Content $filePath -Raw).Trim() }
    return ''
}

function Get-ZipVersion {
    if (-not (Test-Path $zipPath)) { return '' }
    return "$((Get-Item $zipPath).Length)"
}

function Test-PythonReady {
    if (-not (Test-Path $pythonExe)) { return $false }
    try {
        $null = & $pythonExe -c 'pass' 2>&1
        if ($LASTEXITCODE -eq 0) { return $true }
    } catch {}
    return $false
}

function Test-VersionMatch {
    if (-not (Test-Path $versionFile)) { return $false }
    $saved = (Get-Content $versionFile -Raw).Trim()
    $current = Get-ZipVersion
    return ($saved -eq $current -and $saved -ne '')
}

function Do-Status {
    Write-Log '>>> 检查 Python 运行环境...'

    $pythonOk = Test-PythonReady
    $versionOk = Test-VersionMatch

    if ($pythonOk) {
        try {
            $ver = & $pythonExe -c 'import sys; print(sys.version)' 2>&1
            Write-Log "  [OK] Python 已安装: $ver"
        } catch {
            Write-Log '  [OK] Python 可执行文件存在'
        }
    } else {
        Write-Log '  [缺失] Python 可执行文件不存在或无法运行'
    }

    if ($versionOk) {
        $saved = (Get-Content $versionFile -Raw).Trim()
        Write-Log "  [OK] 版本指纹匹配: $saved"
    } else {
        $current = Get-ZipVersion
        if ($current -eq '') {
            Write-Log '  [缺失] 未找到 python-runtime.zip'
        } else {
            Write-Log "  [需更新] 版本指纹不匹配 (zip: $current)"
        }
    }

    if ($pythonOk -and $versionOk) {
        Write-Status -Name 'all' -Value 'ok'
        Write-Log '>>> 环境完好，无需更新'
    } else {
        Write-Status -Name 'all' -Value 'needs_update'
        Write-Log '>>> 环境需要更新'
    }
}

function Do-Install {
    if (-not (Test-Path $zipPath)) {
        Write-Log '[致命错误] 未找到 python-runtime.zip，无法安装'
        exit 1
    }

    $pythonOk = Test-PythonReady
    $versionOk = Test-VersionMatch

    if ($pythonOk -and $versionOk) {
        Write-Log '[OK] Python 运行环境已是最新版本'
        return
    }

    Write-Log '>>> 开始安装 Python 运行环境...'

    if (Test-Path $PythonDir) {
        Write-Log '  清理旧运行环境...'
        & "C:\Program Files\python\python.exe" -c "import shutil; shutil.rmtree(r'$PythonDir', ignore_errors=True)"
        New-Item -ItemType Directory -Path $PythonDir -Force | Out-Null
    }

    Write-Log '  解压 python-runtime.zip...'
    tar -xf "$zipPath" -C "$PythonDir"

    if (-not (Test-Path $pythonExe)) {
        Write-Log '[致命错误] 解压后未找到 python.exe'
        exit 1
    }

    $zipVersion = Get-ZipVersion
    Set-Content -Path $versionFile -Value $zipVersion -Encoding UTF8
    Write-Log "  版本指纹已写入: $zipVersion"

    $ver = & $pythonExe -c 'import sys; print(sys.version)' 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Log "[致命错误] Python 运行环境验证失败: $ver"
        exit 1
    }

    Write-Log ">>> Python 运行环境安装完成: $ver"
}

function Do-All {
    Do-Status
    $allStatus = Read-Status -Name 'all'
    if ($allStatus -eq 'ok') { return }

    Do-Install

    $finalVer = & $pythonExe -c 'import sys; print(sys.version)' 2>&1
    Write-Log ''
    Write-Log '========================================'
    Write-Log "  Python: $finalVer"
    Write-Log '  运行环境配置完成！'
    Write-Log '========================================'
}

switch ($Action) {
    'status'  { Do-Status }
    'python'  { Do-Install }
    default   { Do-All }
}
