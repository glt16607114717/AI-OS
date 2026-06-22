# AI-OS Build Script
$ErrorActionPreference = "Continue"
# 脚本位于 tool/scripts/build.ps1，往上一级到 tool/
$projectDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $projectDir

$buildOutDir = Join-Path $projectDir "dist-build"
$cacheDir = Join-Path $projectDir ".cache"

# Cache dirs inside project to avoid sandbox restrictions on C:\Users\...
$env:ELECTRON_BUILDER_CACHE = $cacheDir
$env:ELECTRON_CACHE = $cacheDir
New-Item -ItemType Directory -Force -Path $cacheDir | Out-Null

Write-Host "=== AI-OS Build ===" -ForegroundColor Cyan
Write-Host "Project: $projectDir" -ForegroundColor DarkGray
Write-Host "Output:  $buildOutDir" -ForegroundColor DarkGray
Write-Host "Cache:   $cacheDir" -ForegroundColor DarkGray

# 0. Pre-clean: kill leftover processes + clean output dir
Write-Host "[0/3] Pre-clean..." -ForegroundColor Yellow

# Kill processes that may lock the output dir (AI-OS app, old installer)
$processNames = @("AI-OS", "AI-OS-Setup-*")
foreach ($pn in $processNames) {
    $procs = Get-Process -Name $pn -ErrorAction SilentlyContinue
    foreach ($p in $procs) {
        Write-Host "  Killing $($p.ProcessName) (PID $($p.Id))" -ForegroundColor DarkGray
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    }
}
Start-Sleep -Milliseconds 500

# Force-clean output dir using robocopy empty-mirror trick + fallback
if (Test-Path $buildOutDir) {
    $emptyTmp = Join-Path $env:TEMP "aios-empty-del-$(Get-Random)"
    New-Item -ItemType Directory -Force -Path $emptyTmp | Out-Null
    robocopy $emptyTmp $buildOutDir /MIR /R:1 /W:1 /NFL /NDL /NJH /NJS /NC /NS | Out-Null
    Remove-Item $emptyTmp -Force -Recurse -ErrorAction SilentlyContinue
    Remove-Item $buildOutDir -Force -Recurse -ErrorAction SilentlyContinue
    if (Test-Path $buildOutDir) {
        try { [System.IO.Directory]::Delete($buildOutDir, $true) } catch {}
    }
    if (Test-Path $buildOutDir) {
        Write-Host "  WARNING: cannot clean $buildOutDir, a process may hold a lock" -ForegroundColor Red
        Write-Host "  Please close AI-OS app or Trae editor and retry" -ForegroundColor Red
        exit 1
    }
    Write-Host "  Cleaned old output" -ForegroundColor Gray
}

# 1. Build
Write-Host "[1/3] Building..." -ForegroundColor Yellow
npm run package
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

# 2. Clean: keep only installer exe, remove intermediates
Write-Host "[2/3] Cleaning..." -ForegroundColor Yellow

if (-not (Test-Path $buildOutDir)) {
    Write-Host "  Build output directory not found!" -ForegroundColor Red
    exit 1
}

$keepPattern = "AI-OS-Setup-*.exe"
$tempKeep = Join-Path $env:TEMP "aios-keep-$(Get-Random)"
New-Item -ItemType Directory -Force -Path $tempKeep | Out-Null

# Move installers to temp first
$moved = Get-ChildItem $buildOutDir -Filter $keepPattern -ErrorAction SilentlyContinue
foreach ($f in $moved) {
    Copy-Item $f.FullName $tempKeep -Force
}

# Wipe output dir via robocopy empty-mirror
$emptyTmp2 = Join-Path $env:TEMP "aios-empty-del2-$(Get-Random)"
New-Item -ItemType Directory -Force -Path $emptyTmp2 | Out-Null
robocopy $emptyTmp2 $buildOutDir /MIR /R:1 /W:1 /NFL /NDL /NJH /NJS /NC /NS | Out-Null
Remove-Item $emptyTmp2 -Force -Recurse -ErrorAction SilentlyContinue

# Restore installers
$kept = Get-ChildItem $tempKeep -Filter $keepPattern -ErrorAction SilentlyContinue
foreach ($f in $kept) {
    Copy-Item $f.FullName $buildOutDir -Force
    Write-Host "  Kept: $($f.Name)" -ForegroundColor Gray
}
Remove-Item $tempKeep -Force -Recurse -ErrorAction SilentlyContinue

# 3. Show result
Write-Host ""
Write-Host "[3/3] Done" -ForegroundColor Green
$result = Get-ChildItem $buildOutDir -Filter "AI-OS-Setup-*.exe" -ErrorAction SilentlyContinue
foreach ($f in $result) {
    $sizeMB = [math]::Round($f.Length / 1MB, 1)
    Write-Host "  $($f.Name) ($sizeMB MB)" -ForegroundColor White
}
