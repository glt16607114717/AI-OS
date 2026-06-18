#!/bin/bash
# 在服务器上安装 Go 并编译
set -e

echo "=== 1. 检查 Go ==="
if [ -f /usr/local/go/bin/go ]; then
    echo "Go already installed"
    /usr/local/go/bin/go version
else
    echo "Installing Go 1.24.4..."
    wget -q https://go.dev/dl/go1.24.4.linux-amd64.tar.gz -O /tmp/go.tar.gz
    sudo tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    echo "Go installed"
    /usr/local/go/bin/go version
fi

# 添加到 PATH
grep -q '/usr/local/go/bin' ~/.bashrc || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
export PATH=$PATH:/usr/local/go/bin

echo "=== 2. 创建 systemd service ==="
sudo tee /etc/systemd/system/aios-server.service > /dev/null << 'EOF'
[Unit]
Description=AIOS Go Backend Server
After=network.target mysql.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/ai-os
ExecStart=/opt/ai-os/ai-os-server
Restart=always
RestartSec=5
Environment=DB_HOST=127.0.0.1
Environment=DB_PORT=3306
Environment=DB_USER=root
Environment=DB_PASS=glt01054717@
Environment=DB_NAME=ai_os

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable aios-server
echo "Systemd service created and enabled"

echo "=== Done ==="
