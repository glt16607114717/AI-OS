param(
    [string]$InstallDir = "C:\Program Files\AI-OS"
)

$ErrorActionPreference = "Continue"
$PassCount = 0
$FailCount = 0

function Test-Case {
    param([string]$Name, [bool]$Passed, [string]$Detail = "")
    $icon = if ($Passed) { "  [PASS]" } else { "  [FAIL]" }
    $color = if ($Passed) { "Green" } else { "Red" }
    $msg = "$icon - $Name"
    if ($Detail) { $msg += ": $Detail" }
    Write-Host $msg -ForegroundColor $color
    if ($Passed) { $script:PassCount++ } else { $script:FailCount++ }
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  AI-OS 安装验证测试" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Install: $InstallDir"
Write-Host ""

# ===== Test 1: Python Environment =====
Write-Host "[Test 1] Python Environment" -ForegroundColor Yellow

$PythonExe = Join-Path $InstallDir "runtime\python\python.exe"

if (Test-Path $PythonExe) {
    $version = & $PythonExe --version 2>&1
    Test-Case "python.exe exists" $true "$version"
    Test-Case "Python version 3.12.10" ($version -match "3\.12\.10") "$version"
} else {
    Test-Case "python.exe exists" $false "Not found: $PythonExe"
    Test-Case "Python version 3.12.10" $false "Skipped"
}

# ===== Test 2: Service Status =====
Write-Host ""
Write-Host "[Test 2] Service Status" -ForegroundColor Yellow

$service = Get-Service -Name "AI-OS-Agent" -ErrorAction SilentlyContinue

if ($service) {
    Test-Case "Service registered" $true "Status: $($service.Status)"
    Test-Case "Service running" ($service.Status -eq 'Running') "Status: $($service.Status)"
} else {
    Test-Case "Service registered" $false "Not found"
    Test-Case "Service running" $false "Skipped"
}

# ===== Test 3: Health Check & Build Time =====
Write-Host ""
Write-Host "[Test 3] Health Check & Build Time" -ForegroundColor Yellow

try {
    $response = Invoke-RestMethod -Uri "http://127.0.0.1:18731/health" -TimeoutSec 5 -ErrorAction Stop
    Test-Case "Health endpoint accessible" $true "ok=$($response.ok)"
    Test-Case "Version reported" $true "version=$($response.version)"

    $buildTime = $response.build_time
    if ($buildTime -and $buildTime -ne "unknown") {
        Test-Case "Build time present" $true "build_time=$buildTime"
        try {
            $buildDate = [DateTime]::Parse($buildTime)
            $diff = ((Get-Date) - $buildDate).TotalHours
            $isRecent = $diff -lt 24
            Test-Case "Build is recent (<24h)" $isRecent "Built $([math]::Round($diff,1)) hours ago"
        } catch {
            Test-Case "Build is recent (<24h)" $false "Cannot parse: $buildTime"
        }
    } else {
        Test-Case "Build time present" $false "build_time=$buildTime"
        Test-Case "Build is recent (<24h)" $false "Skipped"
    }
} catch {
    Test-Case "Health endpoint accessible" $false $_.Exception.Message
    Test-Case "Version reported" $false "Skipped"
    Test-Case "Build time present" $false "Skipped"
    Test-Case "Build is recent (<24h)" $false "Skipped"
}

# ===== Test 4: Dependencies =====
Write-Host ""
Write-Host "[Test 4] Python Dependencies" -ForegroundColor Yellow

if (Test-Path $PythonExe) {
    $RequirementsFile = Join-Path $InstallDir "resources\backend\python\requirements.txt"

    if (Test-Path $RequirementsFile) {
        $requirements = Get-Content $RequirementsFile | Where-Object { $_.Trim() -ne "" -and -not $_.StartsWith("#") }
        $total = $requirements.Count
        $missing = @()

        foreach ($req in $requirements) {
            $pkgName = ($req -split "[><=!~\[]")[0].Trim()
            $showResult = & $PythonExe -m pip show $pkgName 2>$null

            if ($LASTEXITCODE -ne 0) {
                $missing += $pkgName
                Write-Host "    [FAIL] $pkgName - not installed" -ForegroundColor Red
            } else {
                $verLine = ($showResult | Where-Object { $_ -match "Version:" }) -replace "Version:\s*", ""
                Write-Host "    [PASS] $pkgName ($verLine)" -ForegroundColor Green
            }
        }

        Test-Case "All $total deps installed" ($missing.Count -eq 0) "Missing: $($missing -join ', ')"
    } else {
        Test-Case "requirements.txt found" $false "Not found: $RequirementsFile"
    }
} else {
    Test-Case "Dependencies check" $false "Skipped (no Python)"
}

# ===== Test 5: Incremental Dep Install (Manual) =====
Write-Host ""
Write-Host "[Test 5] Incremental Dependency Install (MANUAL)" -ForegroundColor Yellow
Write-Host "  Steps:" -ForegroundColor Gray
Write-Host "  1. Add 'httpx>=0.27.0' to $InstallDir\resources\backend\python\requirements.txt" -ForegroundColor White
Write-Host "  2. Run as admin:" -ForegroundColor Gray
Write-Host "     & '$InstallDir\resources\backend\python\install_deps.ps1' -AppDir '$InstallDir'" -ForegroundColor White
Write-Host "  3. Verify only httpx was installed, others show 'already satisfied'" -ForegroundColor Gray
Write-Host "  4. Verify: & '$InstallDir\runtime\python\python.exe' -m pip show httpx" -ForegroundColor White
Write-Host ""

# ===== Test 6: Service Auto-Recovery (Manual) =====
Write-Host "[Test 6] Service Auto-Recovery (MANUAL)" -ForegroundColor Yellow
Write-Host "  Steps:" -ForegroundColor Gray
Write-Host "  1. Get current PID:" -ForegroundColor Gray
Write-Host "     (Invoke-RestMethod http://127.0.0.1:18731/health).pid" -ForegroundColor White
Write-Host "  2. Kill process:" -ForegroundColor Gray
Write-Host "     Stop-Process -Id <PID> -Force" -ForegroundColor White
Write-Host "  3. Wait 30s for WinSW restart" -ForegroundColor Gray
Write-Host "  4. Get new PID - should be different:" -ForegroundColor Gray
Write-Host "     (Invoke-RestMethod http://127.0.0.1:18731/health).pid" -ForegroundColor White
Write-Host ""

# ===== Summary =====
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Results: $PassCount passed, $FailCount failed" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

if ($FailCount -eq 0) {
    Write-Host "  ALL PASSED" -ForegroundColor Green
} else {
    Write-Host "  SOME TESTS FAILED - check above" -ForegroundColor Red
}
Write-Host ""
