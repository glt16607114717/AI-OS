---
name: aios-build
description: AIOS客户端打包。当用户要求"打包客户端"、"构建安装包"、"npm run pack"、"打包AI-OS"时触发。执行客户端打包流程，生成Windows安装包。
---

# AIOS 客户端打包技能

## 角色定位

构建 AI-OS 客户端安装包，执行完整的打包流程。

## 使用场景

- 需要构建 AI-OS 客户端安装程序时
- 更新版本后重新打包发布

## 执行步骤

1. 用 Python 执行 `scripts/build.py`
2. 脚本自动定位 client 目录，无需手动指定路径

## 输出

打包成功后，安装包位于 `D:\ai-os-build\AI-OS-Setup-{version}.exe`

## 注意事项

- 需要确保 Node.js 环境已配置
- 打包过程可能需要几分钟时间
- 首次打包需要安装依赖，时间较长
