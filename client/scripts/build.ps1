# AI-OS Build Script
$projectDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $projectDir

# Output dir is outside project to avoid Trae locking app.asar
$releaseDir = "D:\ai-os-build5"

Write-Host "=== AI-OS Build ===" -ForegroundColor Cyan

# 1. Build
Write-Host "[1/3] Building..." -ForegroundColor Yellow
npm run package
if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

# 2. Clean: keep only installer exe, remove everything else
Write-Host "[2/3] Cleaning..." -ForegroundColor Yellow

Get-ChildItem $releaseDir -ErrorAction SilentlyContinue | Where-Object {
    $_.Name -notlike "AI-OS-Setup-*.exe"
} | ForEach-Object {
    if ($_.PSIsContainer) {
        cmd /c "rmdir /s /q `"$($_.FullName)`"" 2>$null
        if (-not (Test-Path $_.FullName)) {
            Write-Host "  Removed: $($_.Name)/" -ForegroundColor Gray
        } else {
            Write-Host "  Skip (locked): $($_.Name)/" -ForegroundColor DarkGray
        }
    } else {
        Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
        if (-not (Test-Path $_.FullName)) {
            Write-Host "  Removed: $($_.Name)" -ForegroundColor Gray
        } else {
            Write-Host "  Skip (locked): $($_.Name)" -ForegroundColor DarkGray
        }
    }
}

# 3. Show result
Write-Host ""
Write-Host "[3/3] Done" -ForegroundColor Green
Get-ChildItem $releaseDir -Filter "AI-OS-Setup-*.exe" -ErrorAction SilentlyContinue | ForEach-Object {
    $sizeMB = [math]::Round($_.Length / 1MB, 1)
    Write-Host "  $($_.Name) ($sizeMB MB)" -ForegroundColor White
}
