param(
  [Parameter(Mandatory = $true)]
  [string]$AppDir,

  [string]$ProgressFile = ""
)

$ErrorActionPreference = "Stop"

$PythonExe = Join-Path $AppDir "runtime\python\python.exe"
$RequirementsFile = Join-Path $AppDir "resources\backend\python\requirements.txt"
$TargetDir = Join-Path $AppDir "runtime\python\Lib\site-packages"
$PipIndex = "https://mirrors.aliyun.com/pypi/simple"
$DoneFile = Join-Path $AppDir ".progress.done"

function Log {
  param([string]$Message)
  if ($ProgressFile) {
    try { Add-Content -Path $ProgressFile -Value $Message -ErrorAction SilentlyContinue } catch {}
  }
  Write-Host $Message
}

function Run-Pip {
  param([string[]]$PipArgs)
  $savedEAP = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  $output = & $PythonExe -m pip @PipArgs 2>&1
  $exitCode = $LASTEXITCODE
  $ErrorActionPreference = $savedEAP
  foreach ($line in $output) {
    $text = $line.ToString()
    if ($text -match "^\s*(ERROR|error:|Error:|FAILED|Failed|FAIL)") {
      Log "PIP: $text"
    }
  }
  if ($exitCode -ne 0) {
    throw "pip exit code: $exitCode, args: $($PipArgs -join ' ')"
  }
}

function Get-PackageName {
  param([string]$Requirement)
  return ($Requirement -split "[><=!~\[]")[0].Trim()
}

try {
  Log "Checking Python environment..."

  if (-not (Test-Path $PythonExe)) {
    throw "Python not found: $PythonExe"
  }

  if (-not (Test-Path $RequirementsFile)) {
    throw "requirements.txt not found: $RequirementsFile"
  }

  if (-not (Test-Path $TargetDir)) {
    New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null
  }

  Log "Checking pip..."
  $savedEAP = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  $pipCheck = & $PythonExe -m pip --version 2>&1
  $pipReady = ($LASTEXITCODE -eq 0)
  $ErrorActionPreference = $savedEAP

  if (-not $pipReady) {
    Log "Downloading get-pip.py..."
    $getPipPath = Join-Path $env:TEMP "get-pip.py"
    $wc = New-Object System.Net.WebClient
    $wc.Headers.Add("User-Agent", "AI-OS/1.0")
    $wc.DownloadFile("https://bootstrap.pypa.io/get-pip.py", $getPipPath)
    Log "Installing pip via get-pip.py..."
    $savedEAP3 = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    & $PythonExe $getPipPath 2>&1 | ForEach-Object { Log "GETPIP: $_" }
    $getPipExit = $LASTEXITCODE
    $ErrorActionPreference = $savedEAP3
    if ($getPipExit -ne 0) {
      throw "get-pip.py failed with exit code $getPipExit"
    }
    Remove-Item $getPipPath -Force -ErrorAction SilentlyContinue
    Log "pip installed"
  } else {
    Log "pip ready: $pipCheck"
  }

  Log "Upgrading pip..."
  Run-Pip -Args @("install", "--upgrade", "pip", "-i", $PipIndex, "--no-warn-script-location")

  $requirements = Get-Content $RequirementsFile | Where-Object {
    $_.Trim() -ne "" -and -not $_.StartsWith("#")
  }

  $total = ($requirements | Measure-Object).Count
  $current = 0

  Log "Installing $total packages..."

  foreach ($req in $requirements) {
    $current++
    $pkgName = Get-PackageName $req

    $savedEAP2 = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $null = & $PythonExe -m pip show $pkgName --path $TargetDir 2>&1
    $pkgInstalled = ($LASTEXITCODE -eq 0)
    $ErrorActionPreference = $savedEAP2

    if ($pkgInstalled) {
      Log "[$current/$total] $pkgName - already installed"
      continue
    }

    Log "[$current/$total] Installing $pkgName..."
    Run-Pip -Args @("install", $req, "-i", $PipIndex, "--no-warn-script-location", "--target", $TargetDir)
    Log "[$current/$total] $pkgName - done"
  }

  Log "OK:All $total packages ready"
  Set-Content -Path $DoneFile -Value "0" -NoNewline -ErrorAction SilentlyContinue
  exit 0

} catch {
  $errMsg = $_.Exception.Message
  Log "FATAL: $errMsg"
  try { Set-Content -Path $DoneFile -Value "1" -NoNewline -ErrorAction SilentlyContinue } catch {}
  exit 1
}
