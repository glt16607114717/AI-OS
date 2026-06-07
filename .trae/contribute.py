#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Trae AI 配置回流工具
作者: 向海涛 (Shawn)
联系: shawn@nndrobot.com
版本: 4.3.1
功能: 将本地工作区的优质技能和规则回流至 ai-program 配置中心（三级溯源 + 智能差异感知 + 批量选择）

本文件由 init-trae.py 自动生成，已预置以下参数：
- AI_PROGRAM_PATH: D:\\wwwroot\\ai_program
- DEPT: rmp_platform
- ROLE: backend-php

v4.3.1 改进：
- 移除冗余的 backup_file() 机制：回流目标是 Git 管控的脚手架仓库，版本保护已由 Git 提供，
  文件系统级 .bak 备份不仅冗余，还会被后续回流扫描捕获形成污染链
- is_allowed_to_contribute() 增加 .bak 后缀拦截，防止备份残留进入回流候选

v4.3.0 改进：
- 修复致命时序 bug：git_ensure_clean（fetch+reset）现在在扫描差异之前执行，
  确保差异对比基于远端最新状态，避免扫描结果过时
- auto_git_workflow 拆分为 git_ensure_clean + git_create_branch + git_commit_push 三阶段，
  仓库同步前置、分支创建居中、提交推送殿后，时序逻辑无死角

v4.2.0 改进：
- 分支命名精确到秒（YYYYMMDD-HHMMSS），同一天多次回流不再产生分支名冲突

v4.1.1 改进：
- Git 错误诊断提示：权限不足/远程不可达/文件锁定时给出针对性排查方向
- 三级溯源：变更对比覆盖 1_common/部门级/角色级 全层级，不再仅对比角色级
- 层级标注：修改项显示来源层级（← 通用级/部门级/角色级）
- 无变化精简：内容一致的配置项仅显示单行汇总，不再逐项展开
"""

import os
import sys
import subprocess
import shutil
import argparse
import difflib
import hashlib
import json
from pathlib import Path
from datetime import datetime
from dataclasses import dataclass, field
from typing import List, Dict, Optional, Tuple, Literal
from enum import Enum

AI_PROGRAM_PATH = Path("D:\\wwwroot\\ai_program")
DEPT = "rmp_platform"
ROLE = "backend-php"


class Colors:
    """终端颜色配置（自动适配 Windows/macOS/Linux）"""
    _supports_color = True
    try:
        import platform
        if platform.system() == 'Windows':
            try:
                import ctypes
                kernel32 = ctypes.windll.kernel32
                kernel32.SetConsoleMode(kernel32.GetStdHandle(-11), 7)
            except Exception:
                _supports_color = False
    except Exception:
        pass

    HEADER = '\033[95m' if _supports_color else ''
    OKBLUE = '\033[94m' if _supports_color else ''
    OKCYAN = '\033[96m' if _supports_color else ''
    OKGREEN = '\033[92m' if _supports_color else ''
    WARNING = '\033[93m' if _supports_color else ''
    FAIL = '\033[91m' if _supports_color else ''
    ENDC = '\033[0m' if _supports_color else ''
    BOLD = '\033[1m' if _supports_color else ''
    DIM = '\033[90m' if _supports_color else ''


class ChangeStatus(Enum):
    NEW = "新增"
    MODIFIED = "修改"
    UNCHANGED = "无变化"


class ItemType(Enum):
    SKILL = "skill"
    RULE = "rule"
    MCP = "mcp"
    ASSET = "asset"


@dataclass
class ChangeItem:
    name: str
    item_type: ItemType
    status: ChangeStatus
    src_path: Path
    dest_path: Path
    diff_summary: str = ""
    diff_lines: Tuple[int, int] = (0, 0)
    selected: bool = False

    @property
    def type_label(self) -> str:
        labels = {
            ItemType.SKILL: "技能",
            ItemType.RULE: "规则",
            ItemType.MCP: "MCP",
            ItemType.ASSET: "资产",
        }
        return labels[self.item_type]

    @property
    def status_label(self) -> str:
        status_colors = {
            ChangeStatus.NEW: f"{Colors.OKGREEN}[新增]{Colors.ENDC}",
            ChangeStatus.MODIFIED: f"{Colors.WARNING}[修改]{Colors.ENDC}",
            ChangeStatus.UNCHANGED: f"{Colors.DIM}[无变化]{Colors.ENDC}",
        }
        return status_colors[self.status]


def print_header(msg: str) -> None:
    print(f"\n{Colors.HEADER}{'='*60}{Colors.ENDC}")
    print(f"{Colors.HEADER}  {msg}{Colors.ENDC}")
    print(f"{Colors.HEADER}{'='*60}{Colors.ENDC}\n")


def print_success(msg: str) -> None:
    print(f"{Colors.OKGREEN}[✓]{Colors.ENDC} {msg}")


def print_info(msg: str) -> None:
    print(f"{Colors.OKCYAN}[i]{Colors.ENDC} {msg}")


def print_warning(msg: str) -> None:
    print(f"{Colors.WARNING}[!]{Colors.ENDC} {msg}")


def print_error(msg: str) -> None:
    print(f"{Colors.FAIL}[✗]{Colors.ENDC} {msg}")


def ask_confirm(prompt: str, default: str = "n") -> bool:
    default_prompt = "[Y/n]" if default.lower() == "y" else "[y/N]"
    while True:
        try:
            response = input(f"{Colors.OKCYAN}?{Colors.ENDC} {prompt} {default_prompt}: ").strip().lower()
            if not response:
                return default.lower() == "y"
            return response in ("y", "yes", "是")
        except (EOFError, KeyboardInterrupt):
            print()
            return False


def run_git_command(cmd: List[str], cwd: Path) -> Tuple[bool, str, str]:
    try:
        result = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
        stderr = result.stderr
        if result.returncode != 0:
            if "Permission denied" in stderr or "permission" in stderr.lower():
                stderr += "\n  → 权限不足：请检查 SSH 密钥配置 (ssh -T git@git.nndrobot.com) 或目录文件权限"
            elif "Could not read from remote" in stderr:
                stderr += "\n  → 远程仓库无法访问：请检查 SSH 密钥是否已添加到 GitLab，或网络/VPN 连接"
            elif "unable to unlink" in stderr or "Access denied" in stderr:
                stderr += "\n  → 文件写入失败：请检查目录权限 (ls -la) 或是否有其他程序锁定文件"
        return result.returncode == 0, result.stdout, stderr
    except Exception as e:
        return False, "", str(e)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Trae AI 配置回流工具：智能差异感知 + 批量选择",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
使用示例:
  # 交互式回流（智能扫描，批量选择）
  uv run .trae/contribute.py

  # 仅回流技能
  uv run .trae/contribute.py --type skills

  # 仅回流规则
  uv run .trae/contribute.py --type rules

  # 仅回流 MCP 配置
  uv run .trae/contribute.py --type mcp

  # 仅回流业务资产
  uv run .trae/contribute.py --type assets

v4.1.1 改进:
  - Git 错误诊断提示：权限不足/远程不可达/文件锁定时给出针对性排查方向
  - 三级溯源：变更对比覆盖 1_common/部门级/角色级 全层级
  - 层级标注：修改项显示来源层级
  - 无变化精简：仅显示单行汇总
        """
    )
    parser.add_argument("--type", type=str, choices=["skills", "rules", "mcp", "assets", "all"], default="all", help="回流类型")
    return parser.parse_args()


def validate_target_repo(path: Path) -> bool:
    if not (path / "init-trae.py").exists():
        print_error(f"目标路径 '{path}' 不是有效的 ai-program 仓库（未找到 init-trae.py）")
        return False
    return True


def compute_dir_hash(dir_path: Path) -> str:
    hasher = hashlib.md5()
    for file_path in sorted(dir_path.rglob("*")):
        if file_path.is_file():
            hasher.update(file_path.read_bytes())
    return hasher.hexdigest()


def compute_file_hash(file_path: Path) -> str:
    return hashlib.md5(file_path.read_bytes()).hexdigest()


def compute_mcp_service_hash(service_config: dict) -> str:
    normalized = json.dumps(service_config, sort_keys=True, ensure_ascii=False)
    return hashlib.md5(normalized.encode()).hexdigest()


def count_diff_lines(src_file: Path, dest_file: Path) -> Tuple[int, int]:
    try:
        src_lines = set(src_file.read_text(encoding="utf-8").splitlines())
        dest_lines = set(dest_file.read_text(encoding="utf-8").splitlines())
        added = len(src_lines - dest_lines)
        removed = len(dest_lines - src_lines)
        return added, removed
    except Exception:
        return 0, 0


def get_local_skills() -> List[Path]:
    skills_dir = Path.cwd() / ".trae" / "skills"
    if not skills_dir.exists():
        return []
    return [s for s in skills_dir.iterdir() if s.is_dir()]


def get_local_rules() -> List[Path]:
    rules_dir = Path.cwd() / ".trae" / "rules"
    if not rules_dir.exists():
        return []
    return [r for r in rules_dir.iterdir() if r.is_file() and r.suffix == ".md"]


def get_local_mcp() -> Optional[Dict]:
    mcp_path = Path.cwd() / ".trae" / "mcp.json"
    if not mcp_path.exists():
        return None
    try:
        content = mcp_path.read_text(encoding="utf-8")
        config = json.loads(content)
        return config.get("mcpServers", {})
    except Exception as e:
        print_error(f"读取本地 MCP 配置失败: {e}")
        return None


def _resolve_skill_origin(target_repo: Path, dept: str, role: str, skill_name: str) -> Tuple[Path, str]:
    """三级溯源：定位技能在脚手架中的原始层级

    拼装顺序：1_common → 2_departments/{dept} → 2_departments/{dept}/roles/{role}
    后加载的同名技能覆盖先加载的，因此溯源时从角色级往通用级反向查找。

    Returns:
        (origin_path, level_label) - 原始路径和层级标签
    """
    role_dir = target_repo / "2_departments" / dept / "roles" / role / "skills" / skill_name
    if role_dir.exists():
        return role_dir, "角色级"

    dept_dir = target_repo / "2_departments" / dept / "skills" / skill_name
    if dept_dir.exists():
        return dept_dir, "部门级"

    common_dir = target_repo / "1_common" / "skills" / skill_name
    if common_dir.exists():
        return common_dir, "通用级"

    return target_repo / "2_departments" / dept / "roles" / role / "skills" / skill_name, ""


def _resolve_rule_origin(target_repo: Path, dept: str, role: str, rule_name: str) -> Tuple[Path, str]:
    """三级溯源：定位规则文件在脚手架中的原始层级"""
    role_file = target_repo / "2_departments" / dept / "roles" / role / "rules" / rule_name
    if role_file.exists():
        return role_file, "角色级"

    dept_file = target_repo / "2_departments" / dept / "rules" / rule_name
    if dept_file.exists():
        return dept_file, "部门级"

    common_file = target_repo / "1_common" / "rules" / rule_name
    if common_file.exists():
        return common_file, "通用级"

    return target_repo / "2_departments" / dept / "roles" / role / "rules" / rule_name, ""


def _resolve_mcp_origin(target_repo: Path, dept: str, role: str, service_name: str) -> Tuple[Optional[Path], str]:
    """三级溯源：定位 MCP 服务在脚手架中的原始层级"""
    role_mcp = target_repo / "2_departments" / dept / "roles" / role / "mcp" / "custom-mcp.json"
    if role_mcp.exists():
        try:
            services = json.loads(role_mcp.read_text(encoding="utf-8")).get("mcpServers", {})
            if service_name in services:
                return role_mcp, "角色级"
        except Exception:
            pass

    dept_mcp = target_repo / "2_departments" / dept / "mcp" / "custom-mcp.json"
    if dept_mcp.exists():
        try:
            services = json.loads(dept_mcp.read_text(encoding="utf-8")).get("mcpServers", {})
            if service_name in services:
                return dept_mcp, "部门级"
        except Exception:
            pass

    base_mcp = target_repo / "1_common" / "mcp" / "base-mcp.json"
    if base_mcp.exists():
        try:
            services = json.loads(base_mcp.read_text(encoding="utf-8")).get("mcpServers", {})
            if service_name in services:
                return base_mcp, "通用级"
        except Exception:
            pass

    return None, ""


def scan_skill_changes(target_repo: Path, dept: str, role: str) -> List[ChangeItem]:
    changes: List[ChangeItem] = []
    local_skills = get_local_skills()

    for skill in sorted(local_skills):
        origin_path, level_label = _resolve_skill_origin(target_repo, dept, role, skill.name)
        skill_file = skill / "SKILL.md"

        if not origin_path.exists():
            changes.append(ChangeItem(
                name=skill.name,
                item_type=ItemType.SKILL,
                status=ChangeStatus.NEW,
                src_path=skill,
                dest_path=origin_path,
                diff_summary="新技能目录",
            ))
        elif skill_file.exists() and (origin_path / "SKILL.md").exists():
            local_hash = compute_file_hash(skill_file)
            origin_hash = compute_file_hash(origin_path / "SKILL.md")

            if local_hash != origin_hash:
                added, removed = count_diff_lines(skill_file, origin_path / "SKILL.md")
                level_hint = f" ← {level_label}" if level_label else ""
                changes.append(ChangeItem(
                    name=skill.name,
                    item_type=ItemType.SKILL,
                    status=ChangeStatus.MODIFIED,
                    src_path=skill,
                    dest_path=origin_path,
                    diff_summary=f"+{added}/-{removed} 行{level_hint}",
                    diff_lines=(added, removed),
                ))
            else:
                changes.append(ChangeItem(
                    name=skill.name,
                    item_type=ItemType.SKILL,
                    status=ChangeStatus.UNCHANGED,
                    src_path=skill,
                    dest_path=origin_path,
                ))
        else:
            local_hash = compute_dir_hash(skill)
            origin_hash = compute_dir_hash(origin_path)

            if local_hash != origin_hash:
                level_hint = f" ← {level_label}" if level_label else ""
                changes.append(ChangeItem(
                    name=skill.name,
                    item_type=ItemType.SKILL,
                    status=ChangeStatus.MODIFIED,
                    src_path=skill,
                    dest_path=origin_path,
                    diff_summary=f"目录内容有变化{level_hint}",
                ))
            else:
                changes.append(ChangeItem(
                    name=skill.name,
                    item_type=ItemType.SKILL,
                    status=ChangeStatus.UNCHANGED,
                    src_path=skill,
                    dest_path=origin_path,
                ))

    return changes


def scan_rule_changes(target_repo: Path, dept: str, role: str) -> List[ChangeItem]:
    changes: List[ChangeItem] = []
    local_rules = get_local_rules()

    for rule in sorted(local_rules):
        origin_path, level_label = _resolve_rule_origin(target_repo, dept, role, rule.name)

        if not origin_path.exists():
            changes.append(ChangeItem(
                name=rule.name,
                item_type=ItemType.RULE,
                status=ChangeStatus.NEW,
                src_path=rule,
                dest_path=origin_path,
                diff_summary="新规则文件",
            ))
        else:
            local_hash = compute_file_hash(rule)
            origin_hash = compute_file_hash(origin_path)

            if local_hash != origin_hash:
                added, removed = count_diff_lines(rule, origin_path)
                level_hint = f" ← {level_label}" if level_label else ""
                changes.append(ChangeItem(
                    name=rule.name,
                    item_type=ItemType.RULE,
                    status=ChangeStatus.MODIFIED,
                    src_path=rule,
                    dest_path=origin_path,
                    diff_summary=f"+{added}/-{removed} 行{level_hint}",
                    diff_lines=(added, removed),
                ))
            else:
                changes.append(ChangeItem(
                    name=rule.name,
                    item_type=ItemType.RULE,
                    status=ChangeStatus.UNCHANGED,
                    src_path=rule,
                    dest_path=origin_path,
                ))

    return changes


def scan_mcp_changes(target_repo: Path, dept: str, role: str) -> List[ChangeItem]:
    changes: List[ChangeItem] = []
    local_mcp = get_local_mcp()

    if not local_mcp:
        return changes

    for service_name, service_config in local_mcp.items():
        origin_file, level_label = _resolve_mcp_origin(target_repo, dept, role, service_name)

        if origin_file is None:
            dest_path = target_repo / "2_departments" / dept / "roles" / role / "mcp" / "custom-mcp.json"
            changes.append(ChangeItem(
                name=service_name,
                item_type=ItemType.MCP,
                status=ChangeStatus.NEW,
                src_path=Path.cwd() / ".trae" / "mcp.json",
                dest_path=dest_path,
                diff_summary="新 MCP 服务",
            ))
        else:
            try:
                existing_services = json.loads(origin_file.read_text(encoding="utf-8")).get("mcpServers", {})
                if service_name not in existing_services:
                    dest_path = target_repo / "2_departments" / dept / "roles" / role / "mcp" / "custom-mcp.json"
                    changes.append(ChangeItem(
                        name=service_name,
                        item_type=ItemType.MCP,
                        status=ChangeStatus.NEW,
                        src_path=Path.cwd() / ".trae" / "mcp.json",
                        dest_path=dest_path,
                        diff_summary="新 MCP 服务",
                    ))
                else:
                    local_hash = compute_mcp_service_hash(service_config)
                    origin_hash = compute_mcp_service_hash(existing_services[service_name])

                    if local_hash != origin_hash:
                        level_hint = f" ← {level_label}" if level_label else ""
                        changes.append(ChangeItem(
                            name=service_name,
                            item_type=ItemType.MCP,
                            status=ChangeStatus.MODIFIED,
                            src_path=Path.cwd() / ".trae" / "mcp.json",
                            dest_path=origin_file,
                            diff_summary=f"配置有变化{level_hint}",
                        ))
                    else:
                        changes.append(ChangeItem(
                            name=service_name,
                            item_type=ItemType.MCP,
                            status=ChangeStatus.UNCHANGED,
                            src_path=Path.cwd() / ".trae" / "mcp.json",
                            dest_path=origin_file,
                        ))
            except Exception:
                dest_path = target_repo / "2_departments" / dept / "roles" / role / "mcp" / "custom-mcp.json"
                changes.append(ChangeItem(
                    name=service_name,
                    item_type=ItemType.MCP,
                    status=ChangeStatus.NEW,
                    src_path=Path.cwd() / ".trae" / "mcp.json",
                    dest_path=dest_path,
                    diff_summary="新 MCP 服务",
                ))

    return changes


def show_change_summary(changes: List[ChangeItem]) -> None:
    new_items = [c for c in changes if c.status == ChangeStatus.NEW]
    modified_items = [c for c in changes if c.status == ChangeStatus.MODIFIED]
    unchanged_items = [c for c in changes if c.status == ChangeStatus.UNCHANGED]

    print()
    print(f"{Colors.HEADER}{'─'*60}{Colors.ENDC}")
    print(f"{Colors.HEADER}  📋 变更清单{Colors.ENDC}")
    print(f"{Colors.HEADER}{'─'*60}{Colors.ENDC}")

    if new_items:
        print(f"\n{Colors.OKGREEN}✚ 新增 ({len(new_items)}){Colors.ENDC}")
        for item in new_items:
            print(f"   {Colors.OKGREEN}[新增]{Colors.ENDC} {item.type_label}/{item.name} {Colors.DIM}({item.diff_summary}){Colors.ENDC}")

    if modified_items:
        print(f"\n{Colors.WARNING}✎ 修改 ({len(modified_items)}){Colors.ENDC}")
        for item in modified_items:
            print(f"   {Colors.WARNING}[修改]{Colors.ENDC} {item.type_label}/{item.name} {Colors.DIM}({item.diff_summary}){Colors.ENDC}")

    if unchanged_items:
        print(f"\n{Colors.DIM}○ 无变化 ({len(unchanged_items)} 项): {', '.join(f'{c.type_label}/{c.name}' for c in unchanged_items)}{Colors.ENDC}")

    print()
    print(f"{Colors.HEADER}{'─'*60}{Colors.ENDC}")

    total_changes = len(new_items) + len(modified_items)
    if total_changes == 0:
        print(f"\n{Colors.OKGREEN}✓ 所有配置与脚手架一致，无需回流{Colors.ENDC}")
    else:
        print(f"\n{Colors.OKCYAN}共检测到 {total_changes} 项配置与脚手架存在差异{Colors.ENDC}")


def batch_select(changes: List[ChangeItem]) -> List[ChangeItem]:
    selectable = [c for c in changes if c.status != ChangeStatus.UNCHANGED]

    if not selectable:
        return []

    print()
    print(f"{Colors.OKCYAN}请选择要回流的配置项（输入编号，多个用逗号分隔，输入 'all' 全选，'none' 全不选）：{Colors.ENDC}")
    print()

    for i, item in enumerate(selectable, 1):
        status_icon = "✚" if item.status == ChangeStatus.NEW else "✎"
        print(f"  {Colors.BOLD}[{i}]{Colors.ENDC} {status_icon} {item.type_label}/{item.name} {Colors.DIM}({item.diff_summary}){Colors.ENDC}")

    print()
    print(f"  {Colors.DIM}示例: 1,3,5 或 all 或 none{Colors.ENDC}")
    print()

    while True:
        try:
            response = input(f"{Colors.OKCYAN}?{Colors.ENDC} 选择: ").strip().lower()

            if response == "all":
                for item in selectable:
                    item.selected = True
                return selectable

            if response == "none" or response == "":
                return []

            indices = [int(x.strip()) for x in response.split(",") if x.strip().isdigit()]
            valid_indices = [i for i in indices if 1 <= i <= len(selectable)]

            if not valid_indices:
                print_warning("无效输入，请重新选择")
                continue

            selected = []
            for idx in valid_indices:
                selectable[idx - 1].selected = True
                selected.append(selectable[idx - 1])

            return selected

        except (ValueError, EOFError, KeyboardInterrupt):
            print()
            return []


def is_allowed_to_contribute(file_path: Path) -> bool:
    if file_path.name.endswith(".bak"):
        print_warning(f"拒绝回流：{file_path.name} 为备份残留 (.bak)，禁止进入中央仓库！")
        return False

    MAX_SIZE_MB = 5
    if file_path.stat().st_size > MAX_SIZE_MB * 1024 * 1024:
        print_error(f"拒绝回流：{file_path.name} 超过 {MAX_SIZE_MB}MB，禁止进入中央仓库！")
        return False

    BLOCKED_EXTENSIONS = {
        ".psd", ".sketch", ".ai", ".xd", ".pxd",
        ".mp4", ".mov", ".avi", ".mp3", ".wav",
        ".exe", ".dll", ".so", ".dylib", ".bin", ".class",
        ".pt", ".pth", ".onnx", ".safetensors", ".h5",
        ".zip", ".rar", ".7z", ".tar", ".gz",
    }

    if file_path.suffix.lower() in BLOCKED_EXTENSIONS:
        print_warning(f"拒绝回流：{file_path.name} 属于重型资产 ({file_path.suffix})，请使用 OSS/CDN 管理！")
        return False

    return True


def replace_token_to_placeholder(content: str) -> str:
    try:
        data = json.loads(content)
        token_patterns = ["token", "api_key", "access_token", "api-key", "apikey", "secret", "password", "auth"]
        env_keys = {"env", "environment", "environments"}

        def replace_in_env(obj, inside_env=False):
            if isinstance(obj, dict):
                for key in obj:
                    lower_key = key.lower()
                    is_env_scope = inside_env or lower_key in env_keys
                    if is_env_scope and isinstance(obj[key], str) and obj[key]:
                        if any(pattern in lower_key for pattern in token_patterns):
                            obj[key] = f"{{{{YOUR_{key.upper()}}}}}"
                            continue
                    obj[key] = replace_in_env(obj[key], inside_env=is_env_scope)
            elif isinstance(obj, list):
                for i, item in enumerate(obj):
                    obj[i] = replace_in_env(item, inside_env=inside_env)
            return obj

        data = replace_in_env(data)
        return json.dumps(data, indent=2, ensure_ascii=False)
    except Exception as e:
        print_warning(f"token 替换失败: {e}，保持原样")
        return content


def auto_revert_env_path(content_str: str) -> str:
    node_path = shutil.which("node")
    if node_path:
        content_str = content_str.replace(node_path.replace('\\', '\\\\'), "{{NODE_PATH}}")
        content_str = content_str.replace(node_path, "{{NODE_PATH}}")

    uv_path = shutil.which("uv")
    if uv_path:
        content_str = content_str.replace(uv_path.replace('\\', '\\\\'), "uv")
        content_str = content_str.replace(uv_path, "uv")

    return content_str


def execute_sync(selected_items: List[ChangeItem], target_repo: Path) -> int:
    if not selected_items:
        return 0

    print_header("执行回流")

    synced = 0
    for item in selected_items:
        print_info(f"回流: {item.type_label}/{item.name}")

        if item.item_type == ItemType.SKILL:
            if item.dest_path.exists():
                shutil.rmtree(item.dest_path)
            item.dest_path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copytree(item.src_path, item.dest_path)
            print_success(f"  已同步: {item.dest_path.relative_to(target_repo)}")
            synced += 1

        elif item.item_type == ItemType.RULE:
            item.dest_path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(item.src_path, item.dest_path)
            print_success(f"  已同步: {item.dest_path.relative_to(target_repo)}")
            synced += 1

        elif item.item_type == ItemType.MCP:
            service_name = item.name
            local_mcp = get_local_mcp()
            if local_mcp and service_name in local_mcp:
                service_config = local_mcp[service_name]
                item.dest_path.parent.mkdir(parents=True, exist_ok=True)

                content_str = json.dumps({"mcpServers": {service_name: service_config}}, indent=2, ensure_ascii=False)
                content_str = replace_token_to_placeholder(content_str)
                content_str = auto_revert_env_path(content_str)
                new_content = json.loads(content_str)

                if item.dest_path.exists():
                    with open(item.dest_path, 'r', encoding='utf-8') as f:
                        existing_full = json.loads(f.read())
                    existing_full["mcpServers"][service_name] = new_content["mcpServers"][service_name]
                    final_content = existing_full
                else:
                    final_content = new_content

                with open(item.dest_path, 'w', encoding='utf-8') as f:
                    f.write(json.dumps(final_content, indent=2, ensure_ascii=False))

                print_success(f"  已同步: {service_name} -> {item.dest_path.relative_to(target_repo)}")
                synced += 1

    return synced


def sync_assets(target_repo: Path, dept: str, role: str) -> int:
    print_header("回流业务资产 (workspace_assets)")

    local_cwd = Path.cwd()
    scaffold_assets_dir = target_repo / "2_departments" / dept / "roles" / role / "workspace_assets"

    if not scaffold_assets_dir.exists():
        scaffold_assets_dir.mkdir(parents=True, exist_ok=True)
        print_info(f"已在脚手架中创建资产目录: {scaffold_assets_dir.relative_to(target_repo)}")

    print_info("请选择要回流的文件或目录（相对于项目根目录）")
    print_info("输入完毕后输入空行或 'done' 结束")
    print()

    candidates: List[Path] = []
    while True:
        raw = input("  路径: ").strip()
        if raw.lower() in ("", "done", "end", "q"):
            break
        src = local_cwd / raw
        if not src.exists():
            print_warning(f"  路径不存在: {raw}")
            continue
        if src.is_file():
            if not is_allowed_to_contribute(src):
                continue
            candidates.append(src)
            print_success(f"  已添加: {raw}")
        elif src.is_dir():
            rejected = []
            for f in src.rglob("*"):
                if f.is_file():
                    if is_allowed_to_contribute(f):
                        candidates.append(f)
                    else:
                        rejected.append(f)
            print_success(f"  已添加目录: {raw}（{len(candidates)} 个文件通过检查）")
            if rejected:
                print_warning(f"  已拦截 {len(rejected)} 个不合格文件")
        else:
            print_warning(f"  不支持的文件类型: {raw}")

    if not candidates:
        print_info("没有可回流的资产")
        return 0

    print()
    print_info(f"共 {len(candidates)} 个文件待回流:")
    for c in candidates:
        rel = c.relative_to(local_cwd)
        size_kb = c.stat().st_size / 1024
        print(f"    {rel} ({size_kb:.1f}KB)")

    if not ask_confirm("\n  确认回流以上资产？", default="y"):
        print_info("已取消资产回流")
        return 0

    synced = 0
    for src_file in candidates:
        rel_path = src_file.relative_to(local_cwd)
        dest_file = scaffold_assets_dir / rel_path
        dest_file.parent.mkdir(parents=True, exist_ok=True)

        if dest_file.exists():
            if src_file.read_bytes() == dest_file.read_bytes():
                print_info(f"  跳过（内容相同）: {rel_path}")
                continue
            if not ask_confirm(f"  脚手架已存在: {rel_path}，是否覆盖？", default="n"):
                print_info(f"  跳过: {rel_path}")
                continue

        shutil.copy2(src_file, dest_file)
        print_success(f"  已回流: {rel_path}")
        synced += 1

    if synced:
        print_success(f"共回流 {synced} 个业务资产到 {scaffold_assets_dir.relative_to(target_repo)}")
    else:
        print_info("没有新资产被回流")

    return synced


@dataclass
class GitContext:
    branch_name: str
    orig_branch: str


def git_ensure_clean(target_repo: Path) -> bool:
    print_header("同步 ai-program 仓库到最新")

    success, current_branch, _ = run_git_command(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"], target_repo
    )
    if not success:
        current_branch = "master"

    print_info("Step 1: 获取最新远程信息...")
    success, stdout, stderr = run_git_command(["git", "fetch", "origin"], target_repo)
    if not success:
        print_error(f"git fetch 失败: {stderr}")
        print_warning("网络可能不稳定，请检查 VPN 或网络连接后重试")
        print_info("将基于本地仓库当前状态进行差异扫描（结果可能不准确）")
        return False
    print_success("已获取最新远程信息")

    if current_branch != "master":
        print_info(f"\n当前在分支: {current_branch}，切回 master...")
        success, _, stderr = run_git_command(["git", "checkout", "master"], target_repo)
        if not success:
            print_warning(f"切回 master 失败: {stderr}，继续在当前分支操作")

    print_info("\nStep 2: 强制重置到 origin/master...")
    success, stdout, stderr = run_git_command(["git", "reset", "--hard", "origin/master"], target_repo)
    if not success:
        print_error(f"git reset 失败: {stderr}")
        print_warning("将基于本地仓库当前状态进行差异扫描（结果可能不准确）")
        return False
    print_success("已同步到 origin/master 最新版本")

    return True


def git_create_branch(target_repo: Path, dept: str, role: str) -> Optional[GitContext]:
    print_header("创建贡献分支")

    branch_name = f"contribute/{dept}-{role}-{datetime.now().strftime('%Y%m%d-%H%M%S')}"

    success, orig_branch, _ = run_git_command(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"], target_repo
    )
    if not success:
        orig_branch = "master"

    print_info(f"目标分支: {branch_name}")
    print_info(f"当前分支: {orig_branch}（操作失败时会自动回退到此分支）")

    success, stdout, stderr = run_git_command(["git", "checkout", "-b", branch_name], target_repo)
    if not success:
        print_error(f"创建分支失败: {stderr}")
        _rollback(target_repo, orig_branch, branch_name)
        return None
    print_success(f"已切换到分支: {branch_name}")

    return GitContext(branch_name=branch_name, orig_branch=orig_branch)


def git_commit_push(target_repo: Path, dept: str, role: str, total_synced: int, ctx: GitContext) -> bool:
    print_header("Git 工作流提交与推送")

    print_info("Step 4: 提交变更...")
    commit_msg = f"feat({role}): 回流 {total_synced} 个优质配置"
    success, stdout, stderr = run_git_command(["git", "add", "."], target_repo)
    if not success:
        print_error(f"git add 失败: {stderr}")
        _rollback(target_repo, ctx.orig_branch, ctx.branch_name)
        return False

    success, stdout, stderr = run_git_command(["git", "commit", "-m", commit_msg], target_repo)
    if not success:
        print_error(f"git commit 失败: {stderr}")
        _rollback(target_repo, ctx.orig_branch, ctx.branch_name)
        return False
    print_success("已提交变更")

    print_info(f"\nStep 5: 推送分支并自动创建 MR...")
    push_cmd = [
        "git", "push",
        "-o", "merge_request.create",
        "-o", "merge_request.target=master",
        "-o", f"merge_request.title={commit_msg}",
        "-o", "merge_request.remove_source_branch",
        "origin", ctx.branch_name
    ]
    success, stdout, stderr = run_git_command(push_cmd, target_repo)
    if not success:
        print_warning(f"GitLab Push Options 推送失败，尝试普通推送...")
        success, stdout, stderr = run_git_command(["git", "push", "origin", ctx.branch_name], target_repo)
        if not success:
            print_error(f"git push 失败: {stderr}")
            print_warning("推送失败，但你的提交已保存在本地，不会丢失")
            print()
            print_info("你可以稍后手动推送，执行以下命令：")
            print(f"  cd {target_repo}")
            print(f"  git push -o merge_request.create origin {ctx.branch_name}")
            print()
            print_info(f"如果需要回到原分支 {ctx.orig_branch}，执行：")
            print(f"  git checkout {ctx.orig_branch}")
            return False
        print_success(f"已推送分支: {ctx.branch_name}")
        print()
        print_header("🎉 回流完成")
        print_success(f"共回流 {total_synced} 个配置项")
        print()
        print_info("下一步操作:")
        print(f"  1. 在 GitLab 打开: {target_repo}")
        print(f"  2. 发起 Merge Request: {ctx.branch_name} → master")
        print(f"  3. 等待架构师 Review 合并")
    else:
        print_success(f"已推送分支并自动创建 MR: {ctx.branch_name}")
        mr_url = _extract_mr_url(stdout + stderr)
        print()
        print_header("🎉 回流完成")
        print_success(f"共回流 {total_synced} 个配置项")
        print()
        if mr_url:
            print_success(f"Merge Request 已自动创建: {mr_url}")
        else:
            print_info("Merge Request 已自动创建，请查看 GitLab 获取链接")
        print()
        print_info("下一步操作:")
        print(f"  1. 等待架构师 Review 合并")
        print(f"  2. 合并后源分支将自动删除")

    return True


def _extract_mr_url(output: str) -> Optional[str]:
    import re
    patterns = [
        r'https://git\.nndrobot\.com/[^\s]+/merge_requests/\d+',
        r'https://[^\s]+/merge_requests/\d+',
        r'MR:\s*(https://[^\s]+)',
    ]
    for pattern in patterns:
        match = re.search(pattern, output)
        if match:
            return match.group(0)
    return None


def _rollback(target_repo: Path, orig_branch: str, new_branch: str) -> None:
    print()
    print_warning("正在尝试恢复到操作前的状态...")

    success, _, stderr = run_git_command(
        ["git", "checkout", orig_branch], target_repo
    )
    if success:
        print_success(f"已切回原分支: {orig_branch}")
    else:
        print_error(f"自动切回失败: {stderr}")
        print_info("请手动执行以下命令恢复：")
        print(f"  cd {target_repo}")
        print(f"  git checkout {orig_branch}")

    success, branch_list, _ = run_git_command(
        ["git", "branch"], target_repo
    )
    if success and new_branch in branch_list:
        run_git_command(["git", "branch", "-D", new_branch], target_repo)
        print_info(f"已清理未完成的贡献分支: {new_branch}")


def main() -> None:
    args = parse_args()

    print_header(f"Trae AI 配置回流工具 v4.3.1 (三级溯源 + 智能差异感知)")

    target_repo = AI_PROGRAM_PATH.resolve()
    if not validate_target_repo(target_repo):
        sys.exit(1)

    print_info(f"ai-program 仓库: {target_repo}")
    print_info(f"部门: {DEPT}")
    print_info(f"角色: {ROLE}")
    print_info(f"回流类型: {args.type}")
    print()

    git_ensure_clean(target_repo)

    all_changes: List[ChangeItem] = []

    print_info("正在扫描配置差异...")

    if args.type in ("skills", "all"):
        all_changes.extend(scan_skill_changes(target_repo, DEPT, ROLE))

    if args.type in ("rules", "all"):
        all_changes.extend(scan_rule_changes(target_repo, DEPT, ROLE))

    if args.type in ("mcp", "all"):
        all_changes.extend(scan_mcp_changes(target_repo, DEPT, ROLE))

    if args.type in ("assets", "all"):
        pass

    if args.type not in ("assets",) and all_changes:
        show_change_summary(all_changes)

    selectable = [c for c in all_changes if c.status != ChangeStatus.UNCHANGED]

    git_ctx: Optional[GitContext] = None

    if args.type in ("assets",):
        if ask_confirm("是否启动自动 Git 工作流？", default="y"):
            git_ctx = git_create_branch(target_repo, DEPT, ROLE)
            if not git_ctx:
                print_warning("分支创建失败，资产将回流到 master 分支")
        total_synced = sync_assets(target_repo, DEPT, ROLE)
    elif not selectable:
        print_info("没有需要回流的配置项")
        total_synced = 0
    else:
        selected = batch_select(all_changes)

        if not selected:
            print_info("未选择任何配置项，已取消回流")
            return

        print()
        print_info(f"已选择 {len(selected)} 个配置项进行回流:")
        for item in selected:
            print(f"   {item.status_label} {item.type_label}/{item.name}")

        if not ask_confirm("\n确认执行回流？", default="y"):
            print_info("已取消")
            return

        if ask_confirm("是否启动自动 Git 工作流？", default="y"):
            git_ctx = git_create_branch(target_repo, DEPT, ROLE)
            if not git_ctx:
                print_warning("分支创建失败，文件将回流到 master 分支")

        total_synced = execute_sync(selected, target_repo)

    if total_synced > 0 and git_ctx:
        git_commit_push(target_repo, DEPT, ROLE, total_synced, git_ctx)
    elif total_synced > 0:
        print()
        print_info(f"已回流 {total_synced} 个配置项（未启动 Git 工作流，请手动提交推送）")
    else:
        print_header("完成")
        print_info("未进行任何回流操作")


if __name__ == "__main__":
    main()
