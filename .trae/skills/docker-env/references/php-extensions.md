# PHP 扩展管理详细指南

## Layer 缓存机制

Docker 有智能的 Layer 缓存机制：
- Layer 缓存：如果 Dockerfile 某行没改过，就使用缓存
- 依赖缓存：apk 添加的系统包会复用
- 扩展缓存：已经编译好的扩展会复用

**结论**：添加新扩展通常只需要 **10-30 秒**，不是每次都从头开始。

## 添加新扩展（推荐方式）

```bash
cd D:\wwwroot\.dev-env-docker

# 1. 编辑 Dockerfile，添加新扩展
# 2. 重新构建（利用缓存，快速）
docker-compose build php

# 3. 重启PHP容器
docker-compose up -d php

# 4. 验证扩展已安装
docker exec rmp-php php -m | grep <扩展名>
```

## 扩展类型及添加方法

### 1. PHP 官方扩展（使用 docker-php-ext-install）

```dockerfile
RUN apk add --no-cache <扩展名>-dev
RUN docker-php-ext-install <扩展名>
```

**常用扩展示例**：

```dockerfile
RUN docker-php-ext-install bcmath
RUN docker-php-ext-install soap
RUN docker-php-ext-install pdo_pgsql
RUN docker-php-ext-install xml
RUN docker-php-ext-install ctype
```

### 2. PECL 扩展（使用 pecl install）

```dockerfile
RUN pecl install <扩展名>
RUN docker-php-ext-enable <扩展名>
```

**PECL 扩展示例**：

```dockerfile
RUN pecl install xdebug
RUN docker-php-ext-enable xdebug

RUN pecl install imagick
RUN docker-php-ext-enable imagick

RUN pecl install swoole
RUN docker-php-ext-enable swoole
```

### 3. GD 库图片扩展（需要配置）

```dockerfile
RUN docker-php-ext-configure gd --with-freetype --with-jpeg
RUN docker-php-ext-install gd
```

添加 webp 支持：

```dockerfile
RUN apk add --no-cache libwebp-dev
RUN docker-php-ext-configure gd --with-freetype --with-jpeg --with-webp
RUN docker-php-ext-install gd
```

## 构建场景对比

| 场景 | 命令 | 预计时间 | 说明 |
|------|------|----------|------|
| 添加新扩展 | `docker-compose build php` | **10-30秒** | 利用缓存，只编译新扩展 |
| 修改已有扩展配置 | `docker-compose build php` | **1-2分钟** | 重新编译修改的扩展 |
| 换基础镜像版本 | `docker-compose build php` | **5-10分钟** | 需要重新编译所有扩展 |
| 怀疑缓存问题 | `docker-compose build --no-cache php` | **10-20分钟** | 全量重建，清理所有缓存 |

### 什么时候需要 --no-cache？

仅在以下情况使用：
- 扩展安装失败，怀疑是缓存问题
- 换了不同的基础镜像版本
- 系统包源更换（如从阿里云换到腾讯云）

**平时添加扩展不要用 --no-cache，会浪费 10-20 分钟！**

## 验证扩展安装成功

```bash
docker exec rmp-php php -m                    # 查看所有扩展
docker exec rmp-php php -m | grep <扩展名>    # 查看特定扩展
docker exec rmp-php php -i | grep <扩展名>    # 查看PHP配置
```

## 回滚扩展修改

```bash
# 1. 回滚 Dockerfile 到之前版本
# 2. 重新构建（使用缓存，快速）
docker-compose build php
# 3. 重启容器
docker-compose up -d php
```

## 当前已安装的扩展

**核心扩展**：

- gd - 图片处理
- mysqli、pdo_mysql - MySQL数据库
- mbstring - 多字节字符串（中文支持）
- opcache - 性能优化
- zip - Zip压缩
- sockets - Socket通信
- intl - 国际化

**第三方扩展**：

- redis - Redis缓存
- mongodb - MongoDB数据库

**90% 情况下无需再添加扩展。**

## 最佳实践

1. **优先使用缓存**：添加扩展时不用 `--no-cache`
2. **分层合理**：每个扩展单独一层，避免相互影响
3. **并行编译**：使用 `-j$(nproc)` 多核编译（已配置）
4. **验证安装**：每次安装后验证扩展是否正常加载
5. **快速回滚**：出问题可以快速回滚到之前版本
6. **文档记录**：在 Dockerfile 中注释说明每个扩展的用途
7. **不要修改已有的 RUN 命令**：修改已有的 RUN 命令会导致缓存失效，重新编译所有扩展。新增扩展应该**新建独立 RUN 层**

## 实际操作示例

### 示例1：添加 bcmath 扩展

```dockerfile
# 修改 Dockerfile，在已有 docker-php-ext-install 行末尾追加 bcmath
RUN docker-php-ext-configure gd --with-freetype --with-jpeg \
    && docker-php-ext-install -j$(nproc) \
    gd \
    mysqli \
    pdo_mysql \
    mbstring \
    opcache \
    zip \
    sockets \
    intl \
    bcmath  # 新增
```

```bash
docker-compose build php
docker-compose up -d php
docker exec rmp-php php -m | grep bcmath
```

### 示例2：添加 xdebug 扩展

```dockerfile
# 在 Dockerfile 末尾新增独立 RUN 层
RUN pecl install xdebug
RUN docker-php-ext-enable xdebug
```

```bash
docker-compose build php
docker-compose up -d php
docker exec rmp-php php -m | grep xdebug
```
