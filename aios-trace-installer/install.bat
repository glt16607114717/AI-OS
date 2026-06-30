@echo off
setlocal

echo ============================================
echo   AI-OS Trace Hook - Installer
echo ============================================
echo.

set "TRAE_HOME=%USERPROFILE%\.trae-cn"
set "HOOKS_DIR=%TRAE_HOME%\hooks"
set "PS1_TARGET=%HOOKS_DIR%\aios-trace-hook.ps1"
set "JSON_TARGET=%TRAE_HOME%\hooks.json"

REM 1. Create hooks directory
if not exist "%HOOKS_DIR%" mkdir "%HOOKS_DIR%"

REM 2. Backup existing hooks.json to Desktop (copy, not move)
if exist "%JSON_TARGET%" (
    copy /Y "%JSON_TARGET%" "%USERPROFILE%\Desktop\hooks.json.backup" >nul 2>&1
    echo [OK] Backed up existing hooks.json to Desktop
) else (
    echo [-] No existing hooks.json, skip backup
)

REM 3. Copy ps1 script
copy /Y "%~dp0aios-trace-hook.ps1" "%PS1_TARGET%" >nul 2>&1
if %errorlevel% neq 0 (
    echo [FAIL] Failed to copy hook script
    pause
    exit /b 1
)
echo [OK] Hook script installed

REM 4. Generate hooks.json (replace __PATH__ placeholder)
set "PS1_PATH=%USERPROFILE%\.trae-cn\hooks\aios-trace-hook.ps1"
powershell -NoProfile -Command "$c=(Get-Content '%~dp0hooks.json' -Raw -Encoding UTF8).Replace('__PATH__',('%PS1_PATH%').Replace('\','\\'));$e=New-Object Text.UTF8Encoding $false;[IO.File]::WriteAllText('%JSON_TARGET%',$c,$e)"
if %errorlevel% neq 0 (
    echo [FAIL] Failed to generate hooks.json
    pause
    exit /b 1
)
echo [OK] hooks.json configured

echo.
echo ============================================
echo   Done! Please restart Trae IDE to take effect
echo ============================================
pause