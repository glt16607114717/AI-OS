param(
  [Parameter(Mandatory = $true)]
  [string]$AppDir,

  [string]$ProgressFile = ""
)

$ErrorActionPreference = "Continue"

$PythonExe = Join-Path $AppDir "runtime\python\python.exe"
$RequirementsFile = Join-Path $AppDir "resources\backend\python\requirements.txt"
$TargetDir = Join-Path $AppDir "runtime\python\Lib\site-packages"
$PipIndex = "https://mirrors.aliyun.com/pypi/simple"
$DoneFile = Join-Path $AppDir ".progress.done"
$ExitCode = 1

function Write-Progress-Info {
  param([string]$Message)

  if ($ProgressFile) {
    try { Add-Content -Path $ProgressFile -Value $Message -ErrorAction SilentlyContinue } catch {}
  }
  Write-Host $Message
}

function Get-PackageName {
  param([string]$Requirement)
  return ($Requirement -split "[><=!~\[]")[0].Trim()
}

try {
  if (-not (Test-Path $PythonExe)) {
    Write-Progress-Info "ERROR:Python not found"
    return
  }

  if (-not (Test-Path $RequirementsFile)) {
    Write-Progress-Info "ERROR:requirements.txt not found"
    return
  }

  if (-not (Test-Path $TargetDir)) {
    New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null
  }

  $pipCheck = & $PythonExe -m pip --version 2>$null
  if ($LASTEXITCODE -ne 0) {
    Write-Progress-Info "Installing pip..."
    $nugetLibDir = Join-Path (Split-Path $PythonExe -Parent) "Lib"
    $ensurepipPath = Join-Path $nugetLibDir "ensurepip\__main__.py"
    if (Test-Path $ensurepipPath) {
      & $PythonExe $ensurepipPath 2>$null
      if ($LASTEXITCODE -ne 0) {
        Write-Progress-Info "ERROR:pip install failed"
        return
      }
    } else {
      Write-Progress-Info "ERROR:ensurepip not found"
      return
    }
  }

  Write-Progress-Info "Upgrading pip..."
  & $PythonExe -m pip install --upgrade pip -i $PipIndex --no-warn-script-location --target $TargetDir 2>$null | Out-Null

  $requirements = Get-Content $RequirementsFile | Where-Object {
    $_.Trim() -ne "" -and -not $_.StartsWith("#")
  }

  $total = $requirements.Count
  $installed = 0
  $skipped = 0
  $failed = @()
  $current = 0

  foreach ($req in $requirements) {
    $current++
    $pkgName = Get-PackageName $req

    $null = & $PythonExe -m pip show $pkgName --path $TargetDir 2>$null
    if ($LASTEXITCODE -eq 0) {
      $skipped++
      continue
    }

    Write-Progress-Info "Installing $pkgName ($current/$total)..."

    & $PythonExe -m pip install $req -i $PipIndex --no-warn-script-location --target $TargetDir 2>$null | Out-Null

    if ($LASTEXITCODE -eq 0) {
      $installed++
    } else {
      $failed += $pkgName
      Write-Progress-Info "ERROR:Failed to install $pkgName"
    }
  }

  if ($failed.Count -gt 0) {
    Write-Progress-Info "ERROR:Failed: $($failed -join ', ')"
    return
  }

  $msg = "$total packages checked"
  if ($installed -gt 0) { $msg += ", $installed installed" }
  if ($skipped -gt 0) { $msg += ", $skipped up-to-date" }
  Write-Progress-Info "OK:$msg"
  $ExitCode = 0

} catch {
  Write-Progress-Info "ERROR:Unexpected: $($_.Exception.Message)"
} finally {
  try { Set-Content -Path $DoneFile -Value $ExitCode -NoNewline -ErrorAction SilentlyContinue } catch {}
}
