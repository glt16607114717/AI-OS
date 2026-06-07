---
name: breakpoint
description: 后端断点调试。当用户要求"打断点"、"加断点"、"写日志"、"debug"、"查看变量"、"调试"时触发。通过在代码中插入 log_writer 函数输出调试日志到文件，AI自行查阅输出结果。
---

# 后端断点调试

## 角色定位

在 PHP 代码中插入 `log_writer()` 调用，将变量和调试信息写入日志文件，AI 自行查阅输出结果，定位问题。

## 核心规则

1. **插入断点**：在目标位置调用 `log_writer('关键字', $变量)`
2. **查阅输出**：通过读取日志文件查看结果（本地路径或 SSH 到服务器）
3. **无命令不得擅自清除断点**，上线前用户会统一下指令清除所有断点
4. **方法通用**：`log_writer` 适用于所有 PHP 项目（rmp-api、chartsapi 等），定义在各项目的 `app/common.php` 中
5. **如果当前项目没有这个方法**，直接将下面的代码粘贴到该项目的 `app/common.php` 文件末尾

## log_writer 方法代码

```php
if (!function_exists('log_writer')) {
    function log_writer($keyword, $detail)
    {
        $basePath = dirname(__DIR__, 1);
        $logDir = $basePath . '/runtime/debug_log';
        if (!is_dir($logDir)) {
            mkdir($logDir, 0777, true);
        }

        $logFile = $logDir . '/' . date('Ymd') . '.txt';

        if (is_array($detail) || is_object($detail)) {
            $formattedDetail = json_encode($detail, JSON_UNESCAPED_UNICODE | JSON_PRETTY_PRINT);
        } else {
            $formattedDetail = print_r($detail, true);
        }

        $timestamp = date('H:i:s');
        $logContent = "[{$timestamp}] {$keyword}" . PHP_EOL;
        $logContent .= $formattedDetail . PHP_EOL . PHP_EOL;

        $result = file_put_contents($logFile, $logContent, FILE_APPEND);

        return $result !== false;
    }
}
```

## 日志输出位置

| 环境 | 路径（相对于项目根目录） | 查阅方式 |
|------|--------------------------|----------|
| 本地 | `<项目>/runtime/debug_log/YYYYMMDD.txt` | 直接读取文件 |
| 远程服务器 | `/www/wwwroot/<项目>/runtime/debug_log/YYYYMMDD.txt` | SSH cat/tail |

各项目日志路径示例：
- rmp-api → `rmp-api/runtime/debug_log/20260429.txt`
- chartsapi → `chartsapi/runtime/debug_log/20260429.txt`

## 日志格式

```
[14:51:18] 关键字
{"field":"value","field2":"value2"}

[14:51:19] 另一个关键字
some string value

```

每条日志包含：时间（时分秒）+ 关键字 + 空行 + 详细数据 + 空行分隔。

## 使用方式

### 插入断点

在需要调试的位置插入：

```php
log_writer('ORDER_CREATE_PARAMS', $params);
log_writer('SQL_RESULT', $result);
log_writer('USER_INFO', $user->toArray());
```

### 关键字命名规则

- 保持**唯一性**，方便查阅时快速定位
- 建议格式：`模块_操作_变量名`，如 `ORDER_CREATE_PARAMS`、`DELIVERY_QUERY_RESULT`
- 同一文件多处断点用不同关键字区分

### 查阅日志

**本地**：直接读取 `<项目>/runtime/debug_log/当天日期.txt`

**远程服务器**：SSH 到对应服务器执行
```bash
cat /www/wwwroot/rmp-api/runtime/debug_log/20260429.txt
tail -f /www/wwwroot/chartsapi/runtime/debug_log/20260429.txt
```

## 注意事项

- 断点是临时调试手段，**禁止遗忘清理**
- 用户统一下达清除指令时，需搜索整个项目移除所有 `log_writer` 调用
- 不删除 `log_writer` 函数定义本身，只删除调用处
