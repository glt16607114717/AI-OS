param(
    [Parameter(Mandatory=$false)]
    [ValidateSet("all", "nginx", "php", "redis")]
    [string]$Service = "all",

    [Parameter(Mandatory=$false)]
    [switch]$Follow
)

$ErrorActionPreference = "Continue"

$containerMap = @{
    "nginx" = "rmp-nginx"
    "php" = "rmp-php"
    "redis" = "rmp-redis"
}

$envPath = "D:\wwwroot\.dev-env-docker"

Write-Host "`n=== 查看服务日志 ===`n" -ForegroundColor Cyan

if (-not (Test-Path $envPath)) {
    Write-Host "❌ 错误: 环境目录不存在 $envPath" -ForegroundColor Red
    exit 1
}

try {
    cd $envPath

    if ($Service -eq "all") {
        if ($Follow) {
            Write-Host "📋 实时查看所有服务日志（Ctrl+C 退出）:`n" -ForegroundColor Yellow
            docker-compose logs -f
        } else {
            Write-Host "📋 查看所有服务最新日志:`n" -ForegroundColor Yellow
            docker-compose logs --tail=50
        }
    } else {
        $container = $containerMap[$Service]
        if (-not $container) {
            Write-Host "❌ 错误: 未知服务 $Service" -ForegroundColor Red
            exit 1
        }

        if ($Follow) {
            Write-Host "📋 实时查看 $Service 日志（Ctrl+C 退出）:`n" -ForegroundColor Yellow
            docker logs -f $container
        } else {
            Write-Host "📋 查看 $Service 最新日志:`n" -ForegroundColor Yellow
            docker logs --tail=50 $container
        }
    }

} catch {
    Write-Host "`n❌ 发生错误: $($_.Exception.Message)`n" -ForegroundColor Red
    exit 1
}