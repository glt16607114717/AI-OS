param(
  [Parameter(Mandatory = $true)]
  [string]$AppDir,

  [string]$ProgressFile = ""
)

$ErrorActionPreference = "Stop"

$EmbedUrl = "https://npmmirror.com/mirrors/python/3.12.10/python-3.12.10-embed-amd64.zip"
$NugetUrl = "https://registry.npmmirror.com/-/binary/python/3.12.10/python-3.12.10-amd64.zip"
$RequiredVersion = "3.12.10"

$TempDir = Join-Path $env:TEMP "ai-os-python-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
$PythonDir = Join-Path $AppDir "runtime\python"
$PipIndex = "https://mirrors.aliyun.com/pypi/simple"

function Write-Progress-Info {
  param([string]$Message)

  if ($ProgressFile -and (Test-Path $ProgressFile)) {
    Add-Content -Path $ProgressFile -Value $Message
  }
  Write-Host $Message
}

function Download-File {
  param(
    [string]$Url,
    [string]$OutputPath,
    [string]$Name,
    [int]$TimeoutSeconds = 300
  )

  $TempPath = "$OutputPath.tmp"
  $MaxRetries = 3
  $RetryDelay = 5

  for ($i = 1; $i -le $MaxRetries; $i++) {
    try {
      Write-Progress-Info "DOWNLOAD:正在下载 $Name (尝试 $i/$MaxRetries)..."

      $webClient = New-Object System.Net.WebClient
      $webClient.Headers.Add("User-Agent", "AI-OS-Installer/1.0")

      if (Test-Path $TempPath) {
        $existingSize = (Get-Item $TempPath).Length
        $webClient.Headers.Add("Range", "bytes=$existingSize-")
        Write-Progress-Info "DOWNLOAD:检测到未完成下载，继续从 $([math]::Round($existingSize/1MB,2)) MB 继续"
      }

      $downloadStartTime = Get-Date

      Register-ObjectEvent -InputObject $webClient -EventName DownloadProgressChanged -SourceIdentifier WebClient.DownloadProgressChanged -Action {
        $percentComplete = $EventArgs.ProgressPercentage
        $bytesReceived = $EventArgs.BytesReceived
        $totalBytes = $EventArgs.TotalBytesToReceive
        $speed = [math]::Round($bytesReceived / ((Get-Date).Subtract($script:downloadStartTime).TotalSeconds + 0.1) / 1MB, 2)
        $totalMB = [math]::Round($totalBytes / 1MB, 2)
        $receivedMB = [math]::Round($bytesReceived / 1MB, 2)

        Write-Progress-Info "DOWNLOAD:$Name - $percentComplete% ($receivedMB MB / $totalMB MB) - 速度: $speed MB/s"
      }.GetNewClosure() | Out-Null

      $script:downloadStartTime = Get-Date

      if (Test-Path $TempPath) {
        $webClient.DownloadFileAsync($Url, $TempPath)
      } else {
        $webClient.DownloadFileAsync($Url, $TempPath)
      }

      $timeoutTimer = [Diagnostics.Stopwatch]::StartNew()
      while ($webClient.IsBusy -and $timeoutTimer.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
        Start-Sleep -Milliseconds 100
      }

      $timeoutTimer.Stop()

      if ($webClient.IsBusy) {
        $webClient.CancelAsync()
        throw "下载超时（$TimeoutSeconds 秒）"
      }

      Unregister-Event -SourceIdentifier WebClient.DownloadProgressChanged -ErrorAction SilentlyContinue

      if (Test-Path $TempPath) {
        Move-Item -Path $TempPath -Destination $OutputPath -Force
        $fileSize = [math]::Round((Get-Item $OutputPath).Length / 1MB, 2)
        Write-Progress-Info "DOWNLOAD:$Name 下载完成 ($fileSize MB)"
        return $true
      } else {
        throw "下载文件不存在"
      }
    } catch {
      Write-Progress-Info "DOWNLOAD:下载失败: $_"
      if ($i -lt $MaxRetries) {
        Write-Progress-Info "DOWNLOAD:等待 $RetryDelay 秒后重试..."
        Start-Sleep -Seconds $RetryDelay
      } else {
        throw "下载失败（已达最大重试次数）: $($_.Exception.Message)"
      }
    } finally {
      if ($webClient) {
        $webClient.Dispose()
      }
    }
  }

  return $false
}

function Extract-ZipFile {
  param(
    [string]$ZipPath,
    [string]$DestDir,
    [string]$Name
  )

  Write-Progress-Info "EXTRACT:正在解压 $Name..."

  try {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::ExtractToDirectory($ZipPath, $DestDir)
    Write-Progress-Info "EXTRACT:$Name 解压完成"
  } catch {
    throw "解压失败: $($_.Exception.Message)"
  }
}

function Write-PthConfig {
  param(
    [string]$PythonDir
  )

  $pthPath = Join-Path $PythonDir "python312._pth"
  $pthContent = @"
Lib
Lib\site-packages
.
import site
"@

  Set-Content -Path $pthPath -Value $pthContent -Encoding UTF8
  Write-Progress-Info "PTH:已写入 python312._pth 配置"
}

function Check-PythonVersion {
  param(
    [string]$PythonExe,
    [string]$RequiredVersion
  )

  if (-not (Test-Path $PythonExe)) {
    return $false
  }

  try {
    $versionOutput = & $PythonExe --version 2>&1
    $installedVersion = $versionOutput -replace 'Python ', ''

    return $installedVersion -eq $RequiredVersion
  } catch {
    return $false
  }
}

try {
  Write-Progress-Info "=== 开始下载 Python 运行时 ==="
  Write-Progress-Info "目标目录: $PythonDir"
  Write-Progress-Info "临时目录: $TempDir"

  if (-not (Test-Path $TempDir)) {
    New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
  }

  $PythonExe = Join-Path $PythonDir "python.exe"

  if ((Test-Path $PythonDir) -and (Check-PythonVersion $PythonExe $RequiredVersion)) {
    Write-Progress-Info "SKIP:Python $RequiredVersion 已存在，跳过下载"
    Write-Progress-Info "OK:Python 环境准备完成"
    exit 0
  }

  if (Test-Path $PythonDir) {
    Write-Progress-Info "CLEAN:删除旧的 Python 环境..."
    Remove-Item -Path $PythonDir -Recurse -Force
  }

  $embedPath = Join-Path $TempDir "embed.zip"
  $nugetPath = Join-Path $TempDir "nuget.zip"
  $embedDir = Join-Path $TempDir "embed"
  $nugetDir = Join-Path $TempDir "nuget"

  New-Item -ItemType Directory -Path $embedDir -Force | Out-Null
  New-Item -ItemType Directory -Path $nugetDir -Force | Out-Null

  Write-Progress-Info "STEP:1/5 - 下载 Python embed 包..."
  Download-File -Url $EmbedUrl -OutputPath $embedPath -Name "Python Embed Package" -TimeoutSeconds 600

  Write-Progress-Info "STEP:2/5 - 下载 Python nuget 包（含标准库）..."
  Download-File -Url $NugetUrl -OutputPath $nugetPath -Name "Python Nuget Package" -TimeoutSeconds 600

  Write-Progress-Info "STEP:3/5 - 解压并合并..."
  Extract-ZipFile -ZipPath $embedPath -DestDir $embedDir -Name "Embed 包"
  Extract-ZipFile -ZipPath $nugetPath -DestDir $nugetDir -Name "Nuget 包"

  $nugetLibDir = Join-Path $nugetDir "Lib"
  $embedLibDir = Join-Path $embedDir "Lib"

  if (Test-Path $nugetLibDir) {
    Write-Progress-Info "MERGE:正在合并标准库..."

    Get-ChildItem -Path $nugetLibDir | ForEach-Object {
      $src = $_.FullName
      $dst = Join-Path $embedLibDir $_.Name

      if ($_.PSIsContainer) {
        if (Test-Path $dst) {
          Remove-Item -Path $dst -Recurse -Force
        }
        Copy-Item -Path $src -Destination $dst -Recurse -Force
      } else {
        Copy-Item -Path $src -Destination $dst -Force
      }
    }

    Write-Progress-Info "MERGE:标准库合并完成"
  }

  Write-Progress-Info "STEP:4/5 - 配置 Python 环境..."

  if (Test-Path $PythonDir) {
    Remove-Item -Path $PythonDir -Recurse -Force
  }

  New-Item -ItemType Directory -Path $PythonDir -Force | Out-Null
  Copy-Item -Path "$embedDir\*" -Destination $PythonDir -Recurse -Force

  Write-PthConfig -PythonDir $PythonDir

  Write-Progress-Info "STEP:5/5 - 验证安装..."
  if (Test-Path $PythonExe) {
    $version = & $PythonExe --version 2>&1
    Write-Progress-Info "OK:Python 版本: $version"
  } else {
    throw "Python 可执行文件未找到"
  }

  $sitePackagesDir = Join-Path $PythonDir "Lib\site-packages"
  if (-not (Test-Path $sitePackagesDir)) {
    New-Item -ItemType Directory -Path $sitePackagesDir -Force | Out-Null
  }

  Write-Progress-Info "OK:Python 环境准备完成"
  Write-Progress-Info "CLEAN:清理临时文件..."
  Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue

  exit 0

} catch {
  Write-Progress-Info "ERROR:Python 环境准备失败: $($_.Exception.Message)"
  if (Test-Path $TempDir) {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
  }
  exit 1
}