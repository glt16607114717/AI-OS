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
$DoneFile = Join-Path $AppDir ".progress.done"

function Log {
  param([string]$Message)
  if ($ProgressFile) {
    try { Add-Content -Path $ProgressFile -Value $Message -ErrorAction SilentlyContinue } catch {}
  }
  Write-Host $Message
}

function Download-File {
  param(
    [string]$Url,
    [string]$OutputPath,
    [string]$Label
  )

  Add-Type -AssemblyName System.Net.Http

  $httpClient = New-Object System.Net.Http.HttpClient
  $httpClient.Timeout = [TimeSpan]::FromMinutes(10)
  $httpClient.DefaultRequestHeaders.Add("User-Agent", "AI-OS/1.0")

  $response = $httpClient.GetAsync($Url, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).Result
  $response.EnsureSuccessStatusCode() | Out-Null

  $totalBytes = $response.Content.Headers.ContentLength
  $totalMB = [math]::Round($totalBytes / 1MB, 1)
  Log "$Label ($totalMB MB) downloading..."

  $stream = $response.Content.ReadAsStreamAsync().Result
  $fileStream = [System.IO.File]::Create($OutputPath)

  $buffer = New-Object byte[] (256 * 1024)
  $totalRead = 0
  $lastReportMB = -1

  while ($true) {
    $read = $stream.Read($buffer, 0, $buffer.Length)
    if ($read -le 0) { break }
    $fileStream.Write($buffer, 0, $read)
    $totalRead += $read

    $currentMB = [math]::Floor($totalRead / 1MB)
    if ($currentMB -gt $lastReportMB) {
      $lastReportMB = $currentMB
      $pct = [math]::Round($totalRead / $totalBytes * 100, 1)
      $recvMB = [math]::Round($totalRead / 1MB, 1)
      Log "${Label}: $pct% ($recvMB / $totalMB MB)"
    }
  }

  $fileStream.Close()
  $stream.Close()
  $httpClient.Dispose()

  $actualSize = [math]::Round((Get-Item $OutputPath).Length / 1MB, 1)
  Log "$Label done ($actualSize MB)"
}

try {
  Log "Checking Python environment..."

  $PythonExe = Join-Path $PythonDir "python.exe"

  if ((Test-Path $PythonDir) -and (Test-Path $PythonExe)) {
    $savedEAP = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $ver = & $PythonExe --version 2>&1
    $ErrorActionPreference = $savedEAP

    if ($ver -match $RequiredVersion) {
      Log "OK:Python $RequiredVersion ready, skip download"
      Set-Content -Path $DoneFile -Value "0" -NoNewline -ErrorAction SilentlyContinue
      exit 0
    }
    Log "Version mismatch ($ver), need $RequiredVersion, reinstalling..."
  } else {
    Log "No Python found, downloading..."
  }

  if (Test-Path $PythonDir) {
    Log "Removing old environment..."
    Remove-Item -Path $PythonDir -Recurse -Force
  }

  if (-not (Test-Path $TempDir)) {
    New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
  }

  $embedPath = Join-Path $TempDir "embed.zip"
  $nugetPath = Join-Path $TempDir "nuget.zip"
  $embedDir = Join-Path $TempDir "embed"
  $nugetDir = Join-Path $TempDir "nuget"

  New-Item -ItemType Directory -Path $embedDir -Force | Out-Null
  New-Item -ItemType Directory -Path $nugetDir -Force | Out-Null

  Log "Step 1/5: Downloading Python $RequiredVersion core..."
  Download-File -Url $EmbedUrl -OutputPath $embedPath -Label "Python core"

  Log "Step 2/5: Downloading Python $RequiredVersion stdlib..."
  Download-File -Url $NugetUrl -OutputPath $nugetPath -Label "Python stdlib"

  Log "Step 3/5: Extracting..."
  Add-Type -AssemblyName System.IO.Compression.FileSystem

  [System.IO.Compression.ZipFile]::ExtractToDirectory($embedPath, $embedDir)
  Log "Core extracted"

  [System.IO.Compression.ZipFile]::ExtractToDirectory($nugetPath, $nugetDir)
  Log "Stdlib extracted"

  Log "Step 4/5: Merging stdlib..."
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
    Log "Stdlib merged"
  }

  Log "Step 5/5: Installing to target..."
  if (Test-Path $PythonDir) { Remove-Item -Path $PythonDir -Recurse -Force }
  New-Item -ItemType Directory -Path $PythonDir -Force | Out-Null

  Get-ChildItem -Path $embedDir -Recurse -File | ForEach-Object {
    $relativePath = $_.FullName.Substring($embedDir.Length + 1)
    $dest = Join-Path $PythonDir $relativePath
    $destDir = Split-Path $dest -Parent
    if (-not (Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }
    Copy-Item -Path $_.FullName -Destination $dest -Force
  }

  $pthPath = Join-Path $PythonDir "python312._pth"
  $utf8NoBom = New-Object System.Text.UTF8Encoding $false
  [System.IO.File]::WriteAllText($pthPath, "Lib`r`nLib\site-packages`r`n.`r`nimport site`r`n", $utf8NoBom)

  $spd = Join-Path $PythonDir "Lib\site-packages"
  if (-not (Test-Path $spd)) { New-Item -ItemType Directory -Path $spd -Force | Out-Null }

  $savedEAP2 = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  $verOut = & $PythonExe --version 2>&1
  $ErrorActionPreference = $savedEAP2
  Log "OK:Python $verOut installed"

  Set-Content -Path $DoneFile -Value "0" -NoNewline -ErrorAction SilentlyContinue
  Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
  exit 0

} catch {
  Log "FATAL: $($_.Exception.Message)"
  try { Set-Content -Path $DoneFile -Value "1" -NoNewline -ErrorAction SilentlyContinue } catch {}
  exit 1
}
