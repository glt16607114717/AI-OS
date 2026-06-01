param(
  [Parameter(Mandatory = $true)]
  [string]$AppDir,

  [string]$ProgressFile = ""
)

$ErrorActionPreference = "Continue"

$EmbedUrl = "https://npmmirror.com/mirrors/python/3.12.10/python-3.12.10-embed-amd64.zip"
$NugetUrl = "https://registry.npmmirror.com/-/binary/python/3.12.10/python-3.12.10-amd64.zip"
$RequiredVersion = "3.12.10"

$TempDir = Join-Path $env:TEMP "ai-os-python-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
$PythonDir = Join-Path $AppDir "runtime\python"
$DoneFile = Join-Path $AppDir ".progress.done"
$ExitCode = 1

function Write-Progress-Info {
  param([string]$Message)

  if ($ProgressFile) {
    try { Add-Content -Path $ProgressFile -Value $Message -ErrorAction SilentlyContinue } catch {}
  }
  Write-Host $Message
}

try {
  if (-not (Test-Path $TempDir)) {
    New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
  }

  $PythonExe = Join-Path $PythonDir "python.exe"

  if ((Test-Path $PythonDir) -and (Test-Path $PythonExe)) {
    $ver = & $PythonExe --version 2>&1
    if ($ver -match $RequiredVersion) {
      Write-Progress-Info "OK:Python $RequiredVersion ready"
      $ExitCode = 0
      return
    }
  }

  if (Test-Path $PythonDir) {
    Write-Progress-Info "Removing old Python..."
    Remove-Item -Path $PythonDir -Recurse -Force
  }

  $embedPath = Join-Path $TempDir "embed.zip"
  $nugetPath = Join-Path $TempDir "nuget.zip"
  $embedDir = Join-Path $TempDir "embed"
  $nugetDir = Join-Path $TempDir "nuget"

  New-Item -ItemType Directory -Path $embedDir -Force | Out-Null
  New-Item -ItemType Directory -Path $nugetDir -Force | Out-Null

  Write-Progress-Info "Downloading Python $RequiredVersion core..."
  try {
    $wc = New-Object System.Net.WebClient
    $wc.Headers.Add("User-Agent", "AI-OS/1.0")
    $wc.DownloadFile($EmbedUrl, $embedPath)
    $wc.Dispose()
    $sz = [math]::Round((Get-Item $embedPath).Length / 1MB, 1)
    Write-Progress-Info "Downloaded core package ($sz MB)"
  } catch {
    throw "Download core failed: $($_.Exception.Message)"
  }

  Write-Progress-Info "Downloading Python $RequiredVersion stdlib..."
  try {
    $wc = New-Object System.Net.WebClient
    $wc.Headers.Add("User-Agent", "AI-OS/1.0")
    $wc.DownloadFile($NugetUrl, $nugetPath)
    $wc.Dispose()
    $sz = [math]::Round((Get-Item $nugetPath).Length / 1MB, 1)
    Write-Progress-Info "Downloaded stdlib package ($sz MB)"
  } catch {
    throw "Download stdlib failed: $($_.Exception.Message)"
  }

  Write-Progress-Info "Extracting packages..."
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  [System.IO.Compression.ZipFile]::ExtractToDirectory($embedPath, $embedDir)
  [System.IO.Compression.ZipFile]::ExtractToDirectory($nugetPath, $nugetDir)

  Write-Progress-Info "Merging standard library..."
  $nugetLibDir = Join-Path $nugetDir "Lib"
  $embedLibDir = Join-Path $embedDir "Lib"
  if (Test-Path $nugetLibDir) {
    Get-ChildItem -Path $nugetLibDir | ForEach-Object {
      $dst = Join-Path $embedLibDir $_.Name
      if ($_.PSIsContainer) {
        if (Test-Path $dst) { Remove-Item -Path $dst -Recurse -Force }
        Copy-Item -Path $_.FullName -Destination $dst -Recurse -Force
      } else {
        Copy-Item -Path $_.FullName -Destination $dst -Force
      }
    }
  }

  Write-Progress-Info "Configuring Python runtime..."
  if (Test-Path $PythonDir) { Remove-Item -Path $PythonDir -Recurse -Force }
  New-Item -ItemType Directory -Path $PythonDir -Force | Out-Null
  Copy-Item -Path "$embedDir\*" -Destination $PythonDir -Recurse -Force

  $pthPath = Join-Path $PythonDir "python312._pth"
  $pthContent = "Lib`r`nLib\site-packages`r`n.`r`nimport site`r`n"
  $utf8NoBom = New-Object System.Text.UTF8Encoding $false
  [System.IO.File]::WriteAllText($pthPath, $pthContent, $utf8NoBom)

  $spd = Join-Path $PythonDir "Lib\site-packages"
  if (-not (Test-Path $spd)) { New-Item -ItemType Directory -Path $spd -Force | Out-Null }

  $ver = & $PythonExe --version 2>&1
  Write-Progress-Info "OK:Python $ver installed"
  $ExitCode = 0

} catch {
  Write-Progress-Info "ERROR:Python setup failed: $($_.Exception.Message)"
  $ExitCode = 1
} finally {
  try { Set-Content -Path $DoneFile -Value $ExitCode -NoNewline -ErrorAction SilentlyContinue } catch {}
  try { Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue } catch {}
}
