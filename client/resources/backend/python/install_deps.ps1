param(
  [Parameter(Mandatory = $true)]
  [string]$AppDir,

  [string]$ProgressFile = ""
)

$ErrorActionPreference = "Stop"

$PythonExe = Join-Path $AppDir "runtime\python\python.exe"
$RequirementsFile = Join-Path $AppDir "resources\backend\python\requirements.txt"
$InstalledHashFile = Join-Path $AppDir "runtime\python\.installed_deps_hash.txt"
$PipIndex = "https://mirrors.aliyun.com/pypi/simple"

function Write-Progress-Info {
  param([string]$Message)

  if ($ProgressFile -and (Test-Path $ProgressFile)) {
    Add-Content -Path $ProgressFile -Value $Message
  }
  Write-Host $Message
}

function Get-FileHash {
  param([string]$FilePath)

  if (-not (Test-Path $FilePath)) {
    return ""
  }

  $hash = Get-FileHash -Path $FilePath -Algorithm MD5
  return $hash.Hash
}

function Install-Pip {
  param([string]$PythonExe)

  Write-Progress-Info "PIP:正在安装 pip..."

  $nugetLibDir = Join-Path (Split-Path $PythonExe -Parent) "Lib"

  if (Test-Path (Join-Path $nugetLibDir "ensurepip")) {
    try {
      & $PythonExe (Join-Path $nugetLibDir "ensurepip\__main__.py") 2>&1 | ForEach-Object {
        Write-Progress-Info "PIP:$_"
      }

      if ($LASTEXITCODE -ne 0) {
        throw "pip 安装失败（退出码: $LASTEXITCODE）"
      }
    } catch {
      throw "pip 安装失败: $($_.Exception.Message)"
    }
  } else {
    throw "未找到 ensurepip，请使用完整版 Python"
  }
}

function Upgrade-Pip {
  param([string]$PythonExe)

  Write-Progress-Info "PIP:正在升级 pip..."

  try {
    & $PythonExe -m pip install --upgrade pip -i $PipIndex --no-warn-script-location 2>&1 | ForEach-Object {
      Write-Progress-Info "PIP:$_"
    }

    if ($LASTEXITCODE -ne 0) {
      throw "pip 升级失败（退出码: $LASTEXITCODE）"
    }
  } catch {
    throw "pip 升级失败: $($_.Exception.Message)"
  }
}

function Get-RequirementsList {
  param([string]$RequirementsFile)

  if (-not (Test-Path $RequirementsFile)) {
    throw "requirements.txt 文件不存在: $RequirementsFile"
  }

  $requirements = Get-Content $RequirementsFile | Where-Object {
    $_.Trim() -ne "" -and -not $_.StartsWith("#")
  }

  return $requirements
}

function Install-Dependencies {
  param(
    [string]$PythonExe,
    [string]$RequirementsFile
  )

  $requirements = Get-RequirementsList $RequirementsFile
  $total = $requirements.Count
  $current = 0

  Write-Progress-Info "DEPS:准备安装 $total 个依赖包..."

  foreach ($req in $requirements) {
    $current++
    $pkgName = ($req -split "==")[0].Trim()

    Write-Progress-Info "DEPS:[$current/$total] 正在安装 $pkgName..."

    try {
      & $PythonExe -m pip install $req -i $PipIndex --no-warn-script-location 2>&1 | ForEach-Object {
        if ($_ -match "Successfully installed|Requirement already satisfied") {
          Write-Progress-Info "DEPS:[$current/$total] $pkgName - 成功"
        } elseif ($_ -match "Downloading|Collecting") {
          Write-Progress-Info "DEPS:[$current/$total] $pkgName - 下载中..."
        }
      }

      if ($LASTEXITCODE -ne 0) {
        throw "$pkgName 安装失败（退出码: $LASTEXITCODE）"
      }

      Write-Progress-Info "DEPS:[$current/$total] $pkgName - 完成"
    } catch {
      throw "$pkgName 安装失败: $($_.Exception.Message)"
    }
  }
}

function Verify-Installation {
  param(
    [string]$PythonExe,
    [string]$RequirementsFile
  )

  Write-Progress-Info "VERIFY:正在验证依赖安装..."

  $requirements = Get-RequirementsList $RequirementsFile
  $failed = @()

  foreach ($req in $requirements) {
    $pkgName = ($req -split "==")[0].Trim()
    $versionSpec = ($req -split "==")[1]

    try {
      $result = & $PythonExe -m pip show $pkgName 2>&1

      if ($LASTEXITCODE -ne 0) {
        $failed += $pkgName
      } elseif ($versionSpec) {
        if ($result -match "Version: (.+)") {
          $installedVersion = $matches[1]

          if ($installedVersion -ne $versionSpec) {
            $failed += "$pkgName (需要: $versionSpec, 已安装: $installedVersion)"
          }
        }
      }
    } catch {
      $failed += $pkgName
    }
  }

  if ($failed.Count -gt 0) {
    throw "依赖验证失败: $($failed -join ', ')"
  }

  Write-Progress-Info "VERIFY:所有依赖验证通过"
}

try {
  Write-Progress-Info "=== 开始安装 Python 依赖 ==="
  Write-Progress-Info "Python: $PythonExe"
  Write-Progress-Info "Requirements: $RequirementsFile"

  if (-not (Test-Path $PythonExe)) {
    throw "Python 不存在，请先安装 Python 环境"
  }

  if (-not (Test-Path $RequirementsFile)) {
    throw "requirements.txt 不存在"
  }

  $currentHash = Get-FileHash $RequirementsFile

  if (Test-Path $InstalledHashFile) {
    $installedHash = Get-Content $InstalledHashFile

    if ($installedHash -eq $currentHash) {
      Write-Progress-Info "SKIP:依赖已安装且版本一致，跳过"
      Write-Progress-Info "OK:依赖安装完成"
      exit 0
    } else {
      Write-Progress-Info "UPDATE:依赖配置已变更，重新安装..."
    }
  }

  $sitePackagesDir = Join-Path (Split-Path $PythonExe -Parent) "Lib\site-packages"
  if (-not (Test-Path $sitePackagesDir)) {
    New-Item -ItemType Directory -Path $sitePackagesDir -Force | Out-Null
  }

  Write-Progress-Info "STEP:1/4 - 安装 pip..."
  Install-Pip -PythonExe $PythonExe

  Write-Progress-Info "STEP:2/4 - 升级 pip..."
  Upgrade-Pip -PythonExe $PythonExe

  Write-Progress-Info "STEP:3/4 - 安装依赖包..."
  Install-Dependencies -PythonExe $PythonExe -RequirementsFile $RequirementsFile

  Write-Progress-Info "STEP:4/4 - 验证安装..."
  Verify-Installation -PythonExe $PythonExe -RequirementsFile $RequirementsFile

  Set-Content -Path $InstalledHashFile -Value $currentHash -Encoding UTF8
  Write-Progress-Info "OK:依赖安装完成"
  Write-Progress-Info "OK:依赖哈希已保存: $currentHash"

  exit 0

} catch {
  Write-Progress-Info "ERROR:依赖安装失败: $($_.Exception.Message)"
  exit 1
}