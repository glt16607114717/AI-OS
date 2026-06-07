# AI-OS Build Script
$projectDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $projectDir

# Output dir is outside project to avoid Trae locking app.asar
$releaseDir = "D:\ai-os-build"

Write-Host "=== AI-OS Build ===" -ForegroundColor Cyan

# 1. Build
Write-Host "[1/3] Building..." -ForegroundColor Yellow
npm run package
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

# 2. Clean intermediate files
Write-Host "[2/3] Cleaning..." -ForegroundColor Yellow

$unpacked = Join-Path $releaseDir "win-unpacked"
if (Test-Path $unpacked) {
    Remove-Item $unpacked -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "  Removed: win-unpacked/" -ForegroundColor Gray
}

Get-ChildItem $releaseDir -Filter "builder-*" -ErrorAction SilentlyContinue | ForEach-Object {
    Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
    Write-Host "  Removed: $($_.Name)" -ForegroundColor Gray
}

Get-ChildItem $releaseDir -Filter "*.blockmap" -ErrorAction SilentlyContinue | ForEach-Object {
    Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
    Write-Host "  Removed: $($_.Name)" -ForegroundColor Gray
}

# 3. Show result
Write-Host ""
Write-Host "[3/3] Done" -ForegroundColor Green
Get-ChildItem $releaseDir -Filter "AI-OS-Setup-*.exe" -ErrorAction SilentlyContinue | ForEach-Object {
    $sizeMB = [math]::Round($_.Length / 1MB, 1)
    Write-Host "  $($_.Name) ($sizeMB MB)" -ForegroundColor White
}
