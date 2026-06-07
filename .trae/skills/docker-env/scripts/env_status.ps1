$ErrorActionPreference = "Continue"

$envPath = "D:\wwwroot\.dev-env-docker"

Write-Host "`n=== Docker开发环境状态检查 ===`n" -ForegroundColor Cyan

if (-not (Test-Path $envPath)) {
    Write-Host "❌ 错误: 环境目录不存在 $envPath" -ForegroundColor Red
    exit 1
}

Write-Host "📍 环境位置: $envPath`n" -ForegroundColor Green

try {
    cd $envPath

    Write-Host "🐳 Docker 状态:" -ForegroundColor Yellow
    $dockerStatus = docker ps 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "   ✅ Docker 运行正常" -ForegroundColor Green
    } else {
        Write-Host "   ❌ Docker 未运行或权限不足" -ForegroundColor Red
        Write-Host "   请启动 Docker Desktop`n" -ForegroundColor Yellow
        exit 1
    }

    Write-Host "`n📦 容器状态:" -ForegroundColor Yellow
    $containers = @("rmp-nginx", "rmp-php", "rmp-redis")
    $allRunning = $true

    foreach ($container in $containers) {
        $status = docker ps --filter "name=$container" --format "{{.Status}}"
        if ($status) {
            Write-Host "   ✅ $container - $status" -ForegroundColor Green
        } else {
            Write-Host "   ❌ $container - 未运行" -ForegroundColor Red
            $allRunning = $false
        }
    }

    if ($allRunning) {
        Write-Host "`n🎉 所有容器正常运行！" -ForegroundColor Green
    } else {
        Write-Host "`n⚠️  部分容器未运行，建议启动环境" -ForegroundColor Yellow
    }

    Write-Host "`n🌐 端口监听状态:" -ForegroundColor Yellow
    $ports = @(("80", "Nginx"), ("9000", "PHP-FPM"), ("6379", "Redis"))

    foreach ($portInfo in $ports) {
        $port, $service = $portInfo
        $listening = netstat -ano | Select-String ":$port\s" | Select-String "LISTENING"
        if ($listening) {
            Write-Host "   ✅ 端口 $port ($service) - 正在监听" -ForegroundColor Green
        } else {
            Write-Host "   ❌ 端口 $port ($service) - 未监听" -ForegroundColor Red
        }
    }

    Write-Host "`n🔗 服务可用性测试:" -ForegroundColor Yellow
    $urls = @(
        ("http://rmp-api.me/admin.php", "rmp-api"),
        ("http://rmp-api-a.me/admin.php", "rmp-api-a"),
        ("http://rmp-api-b.me/admin.php", "rmp-api-b"),
        ("http://rmp-api-c.me/admin.php", "rmp-api-c"),
        ("http://charts-api.me/index.php", "charts-api"),
        ("http://mcp.me/mcp", "mcp")
    )

    foreach ($urlInfo in $urls) {
        $url, $name = $urlInfo
        try {
            $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 5
            $status = if ($response.StatusCode -eq 200) { "✅ 可用" } else { "⚠️  状态码: $($response.StatusCode)" }
            Write-Host "   $status $name - $url" -ForegroundColor $(if ($response.StatusCode -eq 200) { "Green" } else { "Yellow" })
        } catch {
            Write-Host "   ❌ 不可用 $name - $url" -ForegroundColor Red
        }
    }

    Write-Host "`n💾 容器资源使用:" -ForegroundColor Yellow
    try {
        $stats = docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Host $stats
        }
    } catch {
        Write-Host "   无法获取资源使用信息" -ForegroundColor Yellow
    }

} catch {
    Write-Host "`n❌ 发生错误: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host "`n"