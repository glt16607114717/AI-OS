---
name: api-debug
description: 后端接口调试技能。当用户要求"API接口调试"、"接口测试"、"测试API"、"复现Bug"、"验证接口"、"调用接口"时触发。AI通过文件传参执行 Python 脚本完成登录、请求、验证全流程，无需外部工具如ApiPost。不触发场景：代码错误调试、报错排查→structured-debugger。
---

# 后端接口调试技能

## 角色定位

本地接口调试专家，通过文件传参 + 执行 Python 脚本完成接口调试。

---

## 环境账号配置

各环境的 API 地址和登录账号密码：

| 环境 | base_url | 登录账号 | 登录密码 |
|------|----------|---------|---------|
| 本地（主工作区） | `http://rmp-api.me/admin.php` | ca-admin | 123456 |
| 本地（工作区A） | `http://rmp-api-a.me/admin.php` | ca-admin | 123456 |
| 本地（工作区B） | `http://rmp-api-b.me/admin.php` | ca-admin | 123456 |
| 本地（工作区C） | `http://rmp-api-c.me/admin.php` | ca-admin | 123456 |
| 开发 | `http://devapi.nndrobot.com/admin.php` | ca-admin | 123456 |
| 测试 | `http://testapi.nndrobot.com/admin.php` | ca-admin | nnd@2026 |
| 灰度 | `http://grayapi.nndrobot.com/admin.php` | ca-admin | s3&K7!gP2#rT9@xQ5 |
| 正式 | `http://api.nndrobot.com/admin.php` | ca-admin | %v2#AsgQxWP# |

**⚠️ 账号优先级**：
1. **Bug 单/需求描述中的测试账号优先** — 当 Bug 单或需求描述中明确提供了测试账号和密码时，必须使用该账号，不要用默认的 `ca-admin`
2. **默认账号次之** — 没有指定测试账号时，使用上表中的默认账号

---

## 环境切换（.env）

脚本请求的是本地站点（通过 Nginx 域名访问），但**数据库在远程服务器上**。接口写入的数据存在哪个环境，完全由项目 `.env` 文件中的数据库配置决定。

**切换方法**：打开项目根目录 `.env` 文件，在 `[DATABASE_BAK]` 区域下有各环境的数据库配置备份，将目标环境的配置复制到 `[DATABASE]` 区域即可。

**使用前必须检查 `.env` 中的数据库指向：**

1. 读取 `.env` 文件，确认 `[DATABASE]` 区域指向哪个环境
2. 接口执行后用 `mysql` 技能查询数据时，**必须选择与 `.env` 相同的环境**，否则查不到数据
3. 切换环境时需要同时修改 `.env` 和查询脚本的环境参数
4. 操作完成后**务必切回原配置**，避免误操作其他环境数据

---

## 调用方式

**唯一方式**：通过文件传参，AI 将 JSON 参数写入临时文件后传给脚本。

```
1. Write → D:\wwwroot\ai\ai_cache\temp\debug_{时间戳}.json
2. RunCommand → python D:\wwwroot\.trae\skills\api-debug\scripts\debug_api.py D:\wwwroot\ai\ai_cache\temp\debug_{时间戳}.json
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| base_url | ✅ | 站点地址，如 `http://rmp-api-a.me/admin.php`，从上方环境账号配置表选取 |
| account | ✅ | 登录账号 |
| password | ✅ | 登录密码 |
| url | ✅ | 接口路径，从 `/api/` 开始 |
| method | ❌ | 请求方式 `GET`/`POST`/`PUT`/`DELETE`，默认 `GET` |
| params | ❌ | 请求参数（JSON 对象），默认 `{}` |
| login_url | ❌ | 登录接口地址，默认 `{base_url}/api/admin/login` |

### 参数文件示例

```json
{
    "base_url": "http://rmp-api-a.me/admin.php",
    "account": "ca-admin",
    "password": "123456",
    "url": "/api/admin/customer-manage/save",
    "method": "POST",
    "params": {"customer_id": 1, "name": "测试"}
}
```

脚本自动处理：GET 请求用 params 拼接 URL，POST 请求用 JSON body 发送，数组参数无需手动序列化。

---

## 使用流程

1. 确认目标环境和接口地址（优先从控制器 `@route` 标签获取）
2. 从环境账号配置表选取 base_url、account、password
3. 构造 JSON 参数，写入临时文件
4. 执行脚本
5. 读取输出，分析结果
6. 如果返回 HTML 错误页面，脚本会自动提取错误信息

---

## 错误处理原则

脚本自动检测 HTML 响应（接口报错时 ThinkPHP 返回 HTML 错误页面），处理逻辑：先去掉 style/script 块，再 strip_tags，压缩空白，截取前300字符，以 `[接口错误]` 前缀输出。

**核心原则：遇到错误必须解决，不能绕过。**

- 接口返回 `[接口错误]` 说明代码有 bug 或参数有误，必须定位原因并修复
- 常见错误类型：ValidateException（参数校验失败）、CommonException（业务逻辑异常）、SQL语法错误等
- 错误信息中包含文件名和行号，据此定位问题代码
- 不能因为某个接口报错就跳过不测，必须确保所有改动点都能正常调用

---

## 路由地址规则

**优先从控制器方法的 `@route` 标签获取路由地址，就近读取，禁止瞎猜。**

所有控制器方法的 PHPDoc 中已维护 `@route` 标签（如 `@route GET /api/admin/customer-manage/list`），调试接口时直接从目标控制器方法上方读取，无需跨文件查找路由文件。如果 `@route` 标签缺失，再回退查阅路由文件 `app/admin/route/route.php`。

base_url 已包含 `/admin.php`，接口路径从 `/api/admin/` 开始。

---

## 数据库联合验证

接口测试必须关联数据库验证：
1. 接口返回 200 不代表数据写入正确
2. 每次写入操作后，用 Python 脚本查询数据库确认字段值
3. 校验类接口要测试正向和反向场景（空值、格式错误等）

---

## 约束条款

1. **所有参数通过文件传入**，临时文件写入 `D:\wwwroot\ai\ai_cache\temp\`
2. **params 只能用 JSON 对象**（嵌套数组直接写，脚本自动处理序列化）
3. **debug_api.py 永远不需要修改**（只改 stdin 传入的参数）
4. **路由地址优先从控制器 `@route` 标签获取**，缺失时再查阅路由文件
5. **使用前确认环境和数据库**，确保请求和查询指向同一个数据库
