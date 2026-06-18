
---
name: "aios-server-ops"
description: "AIOS 服务器运维操作（SSH免密、服务启停、部署），避免重复踩坑。当需要操作 8.163.127.182 服务器时调用此技能。"
---

# AIOS 服务器运维操作技能

## 一、重要避坑记录（避免重复踩坑）

### 1. SSH 免密连接阿里云

| 坑点 | 原因 | 解决方法 |
|------|------|---------|
| SSH 一直要输密码，明明有 authorized_keys | 阿里云默认开启了 `AuthorizedKeysCommand`，完全忽略本地 `~/.ssh/authorized_keys`，只认阿里云控制台注入的密钥 | 1. 注释掉 `/etc/ssh/sshd_config` 里的 `AuthorizedKeysCommand` 和 `AuthorizedKeysCommandUser`；2. 显式添加 `PubkeyAuthentication yes` 和 `AuthorizedKeysFile .ssh/authorized_keys`；3. 重启 sshd |
| SSH config 改不了 | Trae IDE 只允许操作项目目录下的文件，`C:\Users\guilt\.ssh` 不在白名单 | 用 `-i` 参数直接指定密钥文件，或者用封装好的 Python 脚本 |
| paramiko.Ed25519Key.generate() 报错 | 旧版本 paramiko 不支持 | 改用 `paramiko.RSAKey.generate(2048)` |

### 2. 数据库相关坑

| 坑点 | 原因 | 解决方法 |
|------|------|---------|
| MySQL 连不上，提示 Can't connect to local MySQL server through socket | MySQL server 被卸载了（只剩 client） | `apt install mysql-server-8.0` 重装，**数据会自动复用 `/var/lib/mysql`**，不用备份恢复！ |
| MySQL up 但监听 127.0.0.1 连不上外网 | 阿里云默认 `bind-address = 127.0.0.1` | 改 `/etc/mysql/mysql.conf.d/mysqld.cnf` 为 `bind-address = 0.0.0.0`，重启 mysql |
| 本地连云端 3306 连不上 | ufw 防火墙没放行 | `ufw allow 3306/tcp; ufw allow 18731/tcp; ufw reload` |

### 3. 宝塔相关

- 宝塔安装后会接管 ufw 防火墙规则
- 建议先把核心服务（3306、18731、32514 等）加入白名单
- 系统 MySQL 和宝塔可以共存，不用强制用宝塔的 MySQL

---

## 二、服务器基础信息

| 项 | 值 |
|------|------|
| IP 地址 | 8.163.127.182 |
| SSH 端口 | 443 |
| 用户名 | root |
| MySQL 密码 | glt01054717@ |
| 数据库名 | ai_os |
| Go 后端服务名 | ai-os |
| 后端端口 | 18731 |

---

## 三、一键使用方式

**优先使用封装好的 Python 脚本（永久免密）：**

```powershell
# 进入项目根目录
cd d:\wwwroot\AI\AI-OS

# 执行任意服务器命令
python aios_ssh.py "命令内容"
```

**示例：**
```powershell
# 查看服务状态
python aios_ssh.py "systemctl status ai-os"

# 重启后端
python aios_ssh.py "systemctl restart ai-os"

# 查看数据库
python aios_ssh.py "mysql -uroot -p'glt01054717@' -e 'USE ai_os; SHOW TABLES;'"

# 上传文件
python aios_ssh.py "scp 本地文件 root@8.163.127.182:/opt/ai-os/"  # 用原生 scp 也可以
```

---

## 四、常用运维命令

### 服务管理

```bash
# 查看服务状态
systemctl status ai-os
systemctl status mysql

# 启动/停止/重启
systemctl start/stop/restart ai-os
systemctl start/stop/restart mysql

# 查看日志
journalctl -u ai-os -f
tail -f /var/log/mysql/error.log
```

### 部署 Go 后端

```bash
# 1. 本地编译
cd go-backend
GOOS=linux GOARCH=amd64 go build -o ai-os-server

# 2. 上传到服务器
scp -i C:\Users\guilt\.ssh\aios_ed25519 -P 443 ai-os-server root@8.163.127.182:/opt/ai-os/

# 3. 重启服务
python aios_ssh.py "systemctl restart ai-os"
```

### 防火墙管理

```bash
# 查看状态
ufw status

# 放行端口
ufw allow <端口>/tcp

# 重载
ufw reload
```

---

## 五、如果需要用原生 SSH 连接

```powershell
ssh -i C:\Users\guilt\.ssh\aios_ed25519 -p 443 root@8.163.127.182
```

---

## 六、快速问题排查清单

1. **后端不工作？**
   - 先看日志：`python aios_ssh.py "journalctl -u ai-os -n 20"`
   - 检查 MySQL：`python aios_ssh.py "systemctl is-active mysql"`
   - 检查防火墙：`python aios_ssh.py "ufw status"`

2. **本地连不上后端？**
   - 检查本地 Go 是否在运行
   - 检查 api.ts 配置是否正确（dev 用 localhost）
   - 检查 CORS（后端已配置好，不用改）

3. **数据库连不上？**
   - 先 ping 服务器
   - 检查 3306 是否在 ufw 白名单
   - 检查 bind-address 是否为 0.0.0.0
