$ErrorActionPreference = "Stop"

$envPath = "D:\wwwroot\.dev-env-docker"

Write-Host "`n=== 重启 Docker 开发环境 ===`n" -ForegroundColor Cyan

if (-not (Test-Path $envPath)) {
    Write-Host "❌ 错误: 环境目录不存在 $envPath" -ForegroundColor Red
    exit 1
}

Write-Host "📍 环境位置: $envPath`n" -ForegroundColor Green

try {
    cd $envPath

    Write-Host "🛑 停止所有容器..." -ForegroundColor Yellow
    docker-compose down
    if ($LASTEXITCODE -eq 0) {
        Write-Host "   ✅ 容器已停止" -ForegroundColor Green
    } else {
        Write-Host "   ⚠️  停止容器时出现警告" -ForegroundColor Yellow
    }

    Write-Host "`n🚀 启动所有容器..." -ForegroundColor Yellow
    docker-compose up -d
    if ($LASTEXITCODE -eq 0) {
        Write-Host "   ✅ 容器已启动" -ForegroundColor Green
    } else {
        Write-Host "   ❌ 启动容器失败" -ForegroundColor Red
        exit 1
    }

    Write-Host "`n⏳ 等待服务完全启动..." -ForegroundColor Yellow
    Start-Sleep -Seconds 5

    Write-Host "`n📦 容器状态:" -ForegroundColor Yellow
    docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

    Write-Host "`n✅ 开发环境重启完成！" -ForegroundColor Green

} catch {
    Write-Host "`n❌ 发生错误: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host "`n"