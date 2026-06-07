---
name: rmp-login
description: RMP 系统自动登录。当用户要求"登录"、"自动登录"、"登录 dev/test/gray 环境"或使用预设账号快速登录时触发。支持多环境（dev/test/gray）和多账号（a-j）。
---

# RMP 自动登录

## 触发场景

- "登录 dev 环境"、"登录测试环境"、"登录灰度"
- "用 a 账号登录"、"登录 test b"
- "查看账号"、"列出所有账号"

## 环境与账号

| 环境 | 地址 | 预设账号 |
|------|------|---------|
| dev | `http://develop.nndrobot.com/login` | a: ca-admin, b: zhangyk, c-j: 待配置 |
| test | `https://test.nndrobot.com/login` | a: ca-admin, b-j: 待配置 |
| gray | `https://gray.nndrobot.com/login` | a: ca-admin, b-j: 待配置 |

完整配置见 `scripts/account_config.json`。

## 调用方式

```bash
cd D:\wwwroot\.trae\skills\rmp-login\scripts
node auto_login.js --list              # 查看账号
node auto_login.js dev a               # 登录开发环境 a 账号
node auto_login.js test b              # 登录测试环境 b 账号
node auto_login.js dev a user pass     # 自定义账号密码
```

## 执行流程

1. 解析环境和账号参数
2. 读取 `account_config.json`，未指定账号则显示列表
3. 启动 Chromium（非 headless），执行登录
4. 浏览器保持打开，等待用户操作

## 依赖

```bash
cd D:\wwwroot\.trae\skills\rmp-login\scripts && npm install
```
