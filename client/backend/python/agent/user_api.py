"""
用户管理 API（系统设置 > 账户设置）

连接远程 MySQL，提供用户 CRUD + 登录接口。
首次启动自动建库建表。

@author 桂良涛
"""

import hashlib
import logging
import re
import secrets
from datetime import datetime, timedelta

from fastapi import APIRouter, HTTPException, Header
from pydantic import BaseModel

logger = logging.getLogger("agent")

try:
    import pymysql
except ImportError:
    pymysql = None

# 远程 MySQL 配置
DB_CONFIG = {
    "host": "124.221.220.89",
    "port": 23306,
    "user": "root",
    "password": "glt01054717@",
    "charset": "utf8mb4",
    "connect_timeout": 10,
    "read_timeout": 10,
}
DB_NAME = "ai_os"
TABLE_NAME = "sys_user"
SESSION_TABLE = "sys_session"

router = APIRouter(prefix="/api/system", tags=["system"])

# 会话缓存（内存，重启清空）
_sessions: dict[str, dict] = {}  # token -> {user_id, username, is_admin, expire}
_user_tokens: dict[int, str] = {}  # user_id -> token（单点互踢反向映射）


# --- Models ---

class UserCreate(BaseModel):
    username: str
    password: str
    is_admin: int = 0


class UserUpdate(BaseModel):
    password: str | None = None


class UserAdminToggle(BaseModel):
    is_admin: int  # 0=普通 1=管理员


class UserStatusToggle(BaseModel):
    status: int  # 1=启用 2=停用


class LoginRequest(BaseModel):
    username: str
    password: str


# --- DB Helpers ---

def _get_conn(database: str = DB_NAME):
    """获取 MySQL 连接"""
    if pymysql is None:
        raise HTTPException(status_code=500, detail="pymysql 未安装")
    cfg = dict(DB_CONFIG)
    cfg["database"] = database
    return pymysql.connect(**cfg, cursorclass=pymysql.cursors.DictCursor)


def _hash_password(password: str) -> str:
    """SHA256 哈希密码"""
    return hashlib.sha256(password.encode("utf-8")).hexdigest()


def _get_current_user(authorization: str = Header(None)) -> dict | None:
    """从 Authorization header 解析当前用户"""
    if not authorization:
        return None
    token = authorization.replace("Bearer ", "").strip()
    session = _sessions.get(token)
    if not session:
        return None
    if session.get("expire") and session["expire"] < datetime.now():
        user_id = session.get("user_id")
        del _sessions[token]
        if user_id and _user_tokens.get(user_id) == token:
            del _user_tokens[user_id]
        return None
    return session


def _require_admin(authorization: str = Header(None)) -> dict:
    """要求管理员权限"""
    user = _get_current_user(authorization)
    if not user:
        raise HTTPException(status_code=401, detail="未登录")
    if not user.get("is_admin"):
        raise HTTPException(status_code=403, detail="需要管理员权限")
    return user


def _ensure_db():
    """首次启动时自动建库建表"""
    try:
        conn = pymysql.connect(**DB_CONFIG, cursorclass=pymysql.cursors.DictCursor)
        with conn:
            cur = conn.cursor()
            cur.execute(f"CREATE DATABASE IF NOT EXISTS `{DB_NAME}` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci")
            cur.execute(f"USE `{DB_NAME}`")
            cur.execute(f"""
                CREATE TABLE IF NOT EXISTS `{TABLE_NAME}` (
                    `id` INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
                    `username` VARCHAR(64) NOT NULL UNIQUE COMMENT '用户名（中文）',
                    `password` VARCHAR(128) NOT NULL COMMENT 'SHA256 哈希',
                    `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1=启用 2=停用',
                    `is_admin` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0=普通用户 1=管理员',
                    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
                ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='系统用户表'
            """)
            # 自动升级：旧表可能没有 is_admin 字段
            try:
                cur.execute(f"ALTER TABLE `{TABLE_NAME}` ADD COLUMN `is_admin` TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0=普通用户 1=管理员' AFTER `status`")
                logger.info("[用户管理] 已添加 is_admin 字段")
            except Exception:
                pass  # 字段已存在则忽略
            # 旧表可能有 nickname 字段，删掉
            try:
                cur.execute(f"ALTER TABLE `{TABLE_NAME}` DROP COLUMN `nickname`")
                logger.info("[用户管理] 已移除 nickname 字段")
            except Exception:
                pass  # 字段不存在则忽略
        logger.info(f"[用户管理] 数据库和表已就绪: {DB_NAME}.{TABLE_NAME}")
    except Exception as e:
        logger.error(f"[用户管理] 建库建表失败: {e}")


# --- API Routes ---

@router.post("/login")
async def login(req: LoginRequest):
    """用户登录，返回 token 和用户信息"""
    username = req.username.strip()
    if not username or not req.password:
        raise HTTPException(status_code=400, detail="用户名和密码不能为空")

    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(
                f"SELECT id, username, status, is_admin FROM `{TABLE_NAME}` WHERE username = %s AND password = %s",
                (username, _hash_password(req.password)),
            )
            user = cur.fetchone()
            if not user:
                raise HTTPException(status_code=401, detail="用户名或密码错误")
            if user["status"] != 1:
                raise HTTPException(status_code=403, detail="账号已停用")

            token = secrets.token_hex(32)
            session_data = {
                "user_id": user["id"],
                "username": user["username"],
                "is_admin": bool(user["is_admin"]),
                "expire": datetime.now() + timedelta(days=30),
            }
            # 单点互踢：踢掉该用户的旧会话
            old_token = _user_tokens.get(user["id"])
            if old_token and old_token in _sessions:
                del _sessions[old_token]
                logger.info(f"[用户管理] 用户 {user['username']} 新登录，踢掉旧会话")
            _sessions[token] = session_data
            _user_tokens[user["id"]] = token
            return {"ok": True, "data": {**session_data, "token": token, "expire": session_data["expire"].isoformat()}}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[用户管理] 登录失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/me")
async def get_current_user(authorization: str = Header(None)):
    """获取当前登录用户信息"""
    user = _get_current_user(authorization)
    if not user:
        return {"ok": False, "error": "未登录"}
    return {"ok": True, "data": user}


@router.get("/users")
async def list_users(authorization: str = Header(None)):
    """获取用户列表（仅管理员）"""
    _require_admin(authorization)
    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(f"SELECT id, username, status, is_admin, created_at, updated_at FROM `{TABLE_NAME}` ORDER BY id")
            rows = cur.fetchall()
            return {"ok": True, "data": rows}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[用户管理] 查询用户列表失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/users")
async def create_user(req: UserCreate, authorization: str = Header(None)):
    """创建用户（仅管理员）"""
    _require_admin(authorization)

    username = req.username.strip()
    if not username or len(username) < 2:
        raise HTTPException(status_code=400, detail="用户名至少 2 个字符")
    if not req.password or len(req.password) < 4:
        raise HTTPException(status_code=400, detail="密码至少 4 个字符")

    hashed = _hash_password(req.password)
    is_admin = 1 if req.is_admin else 0

    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(
                f"INSERT INTO `{TABLE_NAME}` (username, password, is_admin) VALUES (%s, %s, %s)",
                (username, hashed, is_admin),
            )
            conn.commit()
            return {"ok": True, "data": {"id": cur.lastrowid, "username": username, "is_admin": bool(is_admin)}}
    except pymysql.err.IntegrityError:
        raise HTTPException(status_code=400, detail="用户名已存在")
    except Exception as e:
        logger.error(f"[用户管理] 创建用户失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.put("/users/{user_id}")
async def update_user(user_id: int, req: UserUpdate, authorization: str = Header(None)):
    """更新用户（密码）"""
    _require_admin(authorization)
    if not req.password:
        raise HTTPException(status_code=400, detail="请提供新密码")

    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(f"SELECT id FROM `{TABLE_NAME}` WHERE id = %s", (user_id,))
            if not cur.fetchone():
                raise HTTPException(status_code=404, detail="用户不存在")
            hashed = _hash_password(req.password)
            cur.execute(f"UPDATE `{TABLE_NAME}` SET password = %s WHERE id = %s", (hashed, user_id))
            conn.commit()
            return {"ok": True}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[用户管理] 更新用户失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.patch("/users/{user_id}/status")
async def toggle_user_status(user_id: int, req: UserStatusToggle, authorization: str = Header(None)):
    """启用/停用用户（仅管理员）"""
    _require_admin(authorization)
    if req.status not in (1, 2):
        raise HTTPException(status_code=400, detail="status 只允许 1(启用) 或 2(停用)")

    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(f"SELECT id FROM `{TABLE_NAME}` WHERE id = %s", (user_id,))
            if not cur.fetchone():
                raise HTTPException(status_code=404, detail="用户不存在")
            cur.execute(f"UPDATE `{TABLE_NAME}` SET status = %s WHERE id = %s", (req.status, user_id))
            conn.commit()
            return {"ok": True}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[用户管理] 切换用户状态失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.patch("/users/{user_id}/admin")
async def toggle_user_admin(user_id: int, req: UserAdminToggle, authorization: str = Header(None)):
    """切换用户管理员身份（仅管理员）"""
    _require_admin(authorization)

    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(f"SELECT id FROM `{TABLE_NAME}` WHERE id = %s", (user_id,))
            if not cur.fetchone():
                raise HTTPException(status_code=404, detail="用户不存在")
            cur.execute(f"UPDATE `{TABLE_NAME}` SET is_admin = %s WHERE id = %s", (req.is_admin, user_id))
            conn.commit()
            return {"ok": True}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[用户管理] 切换管理员失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@router.delete("/users/{user_id}")
async def delete_user(user_id: int, authorization: str = Header(None)):
    """删除用户（仅管理员）"""
    _require_admin(authorization)

    try:
        conn = _get_conn()
        with conn:
            cur = conn.cursor()
            cur.execute(f"SELECT id FROM `{TABLE_NAME}` WHERE id = %s", (user_id,))
            if not cur.fetchone():
                raise HTTPException(status_code=404, detail="用户不存在")
            cur.execute(f"DELETE FROM `{TABLE_NAME}` WHERE id = %s", (user_id,))
            conn.commit()
            return {"ok": True}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[用户管理] 删除用户失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# --- 启动时自动建表 ---
_ensure_db()
