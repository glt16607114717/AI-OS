"""
Git Flow 统一脚本

提供所有 git flow 操作的统一入口，减少代码重复。

用法：python git_flow.py <子命令> <项目路径> [参数...]

子命令：
  feature_start <项目路径> <分支名>      创建 feature 分支
  feature_finish <项目路径> <分支名>     完成 feature 分支
  hotfix_start <项目路径> <分支名>       创建 hotfix 分支
  hotfix_finish <项目路径> <分支名>      完成 hotfix 分支
  deploy <项目路径> <环境> [提交信息]     部署到指定环境
  commit <项目路径> <提交信息>           提交并推送代码
  branch_delete <项目路径> <分支名>       删除分支
  branch_clean <项目路径> [过期天数]       清理过期分支

示例：
  python git_flow.py feature_start D:/wwwroot/rmp-api/rmp-api-a 1011255-report-export
  python git_flow.py deploy D:/wwwroot/rmp-api/rmp-api-a test "修复导出数据缺失"
  python git_flow.py commit D:/wwwroot/rmp-api/rmp-api-a "--bug=1009333 --user=桂良涛  修复导出缺失"
"""
import subprocess
import sys
import os
from datetime import datetime, timedelta


def run_git(project_path: str, *args: str) -> tuple[bool, str]:
    result = subprocess.run(
        ["git"] + list(args),
        cwd=project_path,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    output = result.stdout.strip() or result.stderr.strip()
    return result.returncode == 0, output


def validate_project_path(project_path: str) -> str:
    project_path = os.path.normpath(project_path)
    if not os.path.isdir(project_path):
        print(f"[错误] 项目目录不存在: {project_path}")
        sys.exit(1)
    if not os.path.exists(os.path.join(project_path, ".git")):
        print(f"[错误] 不是Git仓库: {project_path}")
        sys.exit(1)
    return project_path


def check_uncommitted_changes(project_path: str):
    ok, output = run_git(project_path, "status", "--porcelain")
    if not ok:
        print(f"[错误] git status 执行失败: {output}")
        sys.exit(1)
    if output.strip():
        print(f"[错误] 存在未提交的文件，请先提交或暂存：")
        for line in output.strip().splitlines():
            print(f"  {line}")
        sys.exit(1)


def feature_start(project_path: str, branch_name: str):
    project_path = validate_project_path(project_path)
    check_uncommitted_changes(project_path)

    full_branch = f"feature/{branch_name}"

    ok, output = run_git(project_path, "branch", "--list", full_branch)
    if ok and output.strip():
        print(f"[错误] 分支已存在: {full_branch}")
        sys.exit(1)

    ok, output = run_git(project_path, "checkout", "-b", full_branch)
    if not ok:
        print(f"[错误] 创建分支失败: {output}")
        sys.exit(1)

    print(f"[成功] 已创建并切换到分支: {full_branch}")

    ok, output = run_git(project_path, "push", "-u", "origin", full_branch)
    if not ok:
        print(f"[错误] 推送到远程失败: {output}")
        sys.exit(1)
    print(f"已推送到远程")


def feature_finish(project_path: str, branch_name: str):
    project_path = validate_project_path(project_path)

    if branch_name.startswith("feature/"):
        short_name = branch_name[len("feature/"):]
        full_branch = branch_name
    else:
        short_name = branch_name
        full_branch = f"feature/{branch_name}"

    check_uncommitted_changes(project_path)

    ok, output = run_git(project_path, "branch", "--list", full_branch)
    if not ok or not output.strip():
        print(f"[错误] 分支不存在: {full_branch}")
        sys.exit(1)

    print(f"即将执行 feature finish 操作：")
    print(f"  项目：{project_path}")
    print(f"  分支：{full_branch}")
    print(f"  目标：合并到 develop 并保留原分支（-k）")

    confirm = input("确认执行？(y 继续 / 其他取消)：").strip().lower()
    if confirm != "y":
        print("[取消] 操作已取消")
        sys.exit(0)

    ok, output = run_git(project_path, "checkout", "develop")
    if not ok:
        print(f"[错误] 切换到 develop 失败: {output}")
        sys.exit(1)

    ok, output = run_git(project_path, "pull")
    if not ok:
        print(f"[错误] git pull 失败: {output}")
        sys.exit(1)

    result = subprocess.run(
        ["git", "flow", "feature", "finish", "-k", short_name],
        cwd=project_path,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
    )

    if result.returncode == 0:
        print(f"[成功] feature finish 完成")
        ok, push_output = run_git(project_path, "push", "origin", "develop")
        if ok:
            print(f"[成功] develop 已推送到远程")
    else:
        print(f"[错误] feature finish 执行失败")
        sys.exit(1)


def hotfix_start(project_path: str, branch_name: str):
    project_path = validate_project_path(project_path)
    check_uncommitted_changes(project_path)

    full_branch = f"hotfix/{branch_name}"

    ok, output = run_git(project_path, "branch", "--list", full_branch)
    if ok and output.strip():
        print(f"[错误] 分支已存在: {full_branch}")
        sys.exit(1)

    ok, output = run_git(project_path, "checkout", "master")
    if not ok:
        print(f"[错误] 切换到 master 失败: {output}")
        sys.exit(1)

    ok, output = run_git(project_path, "pull")
    if ok:
        print(f"已拉取最新: {output}")

    ok, output = run_git(project_path, "checkout", "-b", full_branch)
    if not ok:
        print(f"[错误] 创建分支失败: {output}")
        sys.exit(1)

    print(f"[成功] 已基于 master 创建并切换到分支: {full_branch}")

    ok, output = run_git(project_path, "push", "-u", "origin", full_branch)
    if not ok:
        print(f"[警告] 推送到远程失败: {output}")


def hotfix_finish(project_path: str, branch_name: str):
    project_path = validate_project_path(project_path)

    if branch_name.startswith("hotfix/"):
        short_name = branch_name[len("hotfix/"):]
        full_branch = branch_name
    else:
        short_name = branch_name
        full_branch = f"hotfix/{branch_name}"

    check_uncommitted_changes(project_path)

    ok, output = run_git(project_path, "branch", "--list", full_branch)
    if not ok or not output.strip():
        print(f"[错误] 分支不存在: {full_branch}")
        sys.exit(1)

    print(f"即将执行 hotfix finish 操作：")
    print(f"  项目：{project_path}")
    print(f"  分支：{full_branch}")
    print(f"  目标：合并到 master（git flow hotfix finish）")

    confirm = input("确认执行？(y 继续 / 其他取消)：").strip().lower()
    if confirm != "y":
        print("[取消] 操作已取消")
        sys.exit(0)

    ok, output = run_git(project_path, "checkout", "master")
    if not ok:
        print(f"[错误] 切换到 master 失败: {output}")
        sys.exit(1)

    ok, output = run_git(project_path, "pull")
    if not ok:
        print(f"[错误] git pull 失败: {output}")
        sys.exit(1)

    result = subprocess.run(
        ["git", "flow", "hotfix", "finish", short_name],
        cwd=project_path,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
    )

    if result.returncode == 0:
        print(f"[成功] hotfix finish 完成")
    else:
        print(f"[错误] hotfix finish 执行失败")
        sys.exit(1)


def deploy(project_path: str, environment: str, commit_msg: str = None):
    DEPLOY_BRANCHES = {
        "dev": "develop_deploy",
        "test": "release_deploy",
        "gray": "gray_deploy",
    }

    DEPLOYABLE_PREFIXES = ["feature/", "hotfix/", "release/"]

    BRANCH_ENV_RULES = {
        "feature/": ["dev", "test", "gray"],
        "hotfix/": ["dev", "test", "gray"],
        "release/": ["gray"],
    }

    if environment not in DEPLOY_BRANCHES:
        print(f"[错误] 无效环境: {environment}")
        print(f"有效环境: {', '.join(DEPLOY_BRANCHES.keys())}")
        sys.exit(1)

    deploy_branch = DEPLOY_BRANCHES[environment]

    project_path = validate_project_path(project_path)

    ok, output = run_git(project_path, "branch", "--show-current")
    if not ok or not output.strip():
        print("[错误] 无法获取当前分支")
        sys.exit(1)

    source_branch = output.strip()

    matched_prefix = None
    for prefix in DEPLOYABLE_PREFIXES:
        if source_branch.startswith(prefix):
            matched_prefix = prefix
            break

    if not matched_prefix:
        print(f"[错误] 当前分支 {source_branch} 不是 feature/*、hotfix/* 或 release/* 分支")
        sys.exit(1)

    if source_branch == deploy_branch:
        print(f"[错误] 不能将 {deploy_branch} 部署到自身")
        sys.exit(1)

    allowed_envs = BRANCH_ENV_RULES[matched_prefix]
    if environment not in allowed_envs:
        print(f"[错误] {matched_prefix.rstrip('/')}/* 分支只能部署到: {', '.join(allowed_envs)}")
        sys.exit(1)

    ok, output = run_git(project_path, "status", "--porcelain")
    if output.strip():
        if not commit_msg:
            print(f"[错误] 存在未提交的文件，必须传入提交信息参数")
            sys.exit(1)

        print(f"[提示] 存在未提交的文件，将自动提交后继续部署：")
        for line in output.strip().splitlines():
            print(f"  {line}")
        commit_confirm = input("确认提交并继续部署？(y=继续 / 其他=中断)：").strip().lower()
        if commit_confirm != "y":
            print("[中断] 部署已取消")
            sys.exit(1)

        run_git(project_path, "add", "-A")
        ok, output = run_git(project_path, "commit", "-m", commit_msg)
        if not ok:
            print(f"[错误] 提交失败: {output}")
            sys.exit(1)
        print(f"已提交: {commit_msg}")

        ok, output = run_git(project_path, "push")
        if not ok:
            print(f"[错误] 推送失败: {output}")
            sys.exit(1)
        print(f"已推送到远程")

    print("=" * 50)
    print(f"即将执行部署操作：")
    print(f"  项目：{os.path.basename(project_path)}")
    print(f"  源分支：{source_branch}")
    print(f"  目标环境：{environment} → {deploy_branch}")
    print("=" * 50)
    confirm = input("确认部署？请输入 y 继续，其他任意键取消：").strip().lower()
    if confirm != "y":
        print("[取消] 操作已取消")
        sys.exit(0)

    ok, output = run_git(project_path, "checkout", deploy_branch)
    if not ok:
        print(f"[错误] 切换到 {deploy_branch} 失败: {output}")
        sys.exit(1)

    ok, output = run_git(project_path, "pull")
    if ok:
        print(f"已拉取最新: {output}")

    print(f"合并 {source_branch} → {deploy_branch}...")
    ok, output = run_git(project_path, "merge", source_branch)
    if not ok:
        print(f"[错误] 合并冲突: {output}")
        sys.exit(1)
    print(f"合并成功，无冲突")

    ok, output = run_git(project_path, "push")
    if not ok:
        print(f"[错误] 推送失败: {output}")
        sys.exit(1)
    print(f"推送 {deploy_branch} 到远程...")

    ok, output = run_git(project_path, "checkout", source_branch)
    if ok:
        print(f"已切回 {source_branch}")

    print(f"\n[完成] {source_branch} 已部署到 {environment} 环境 ({deploy_branch})")


def commit(project_path: str, commit_msg: str):
    ALLOWED_PREFIXES = ["feature/", "hotfix/", "release/"]

    project_path = validate_project_path(project_path)

    ok, output = run_git(project_path, "branch", "--show-current")
    if not ok or not output.strip():
        print("[错误] 无法获取当前分支")
        sys.exit(1)

    current_branch = output.strip()
    if not any(current_branch.startswith(prefix) for prefix in ALLOWED_PREFIXES):
        print(f"[错误] 当前分支 {current_branch} 不是允许的分支类型")
        sys.exit(1)

    ok, output = run_git(project_path, "status", "--porcelain")
    if not ok:
        print(f"[错误] git status 执行失败: {output}")
        sys.exit(1)

    if not output.strip():
        print("[信息] 没有需要提交的文件变更")
        sys.exit(0)

    print(f"[信息] 项目: {os.path.basename(project_path)} | 分支: {current_branch}")
    print(f"[信息] 提交信息: {commit_msg}")

    confirm = input("确认提交并推送？:").strip().lower()
    if confirm != "y":
        print("[取消] 提交已取消")
        sys.exit(0)

    run_git(project_path, "add", "-A")
    run_git(project_path, "commit", "-m", commit_msg)
    run_git(project_path, "push")

    print("[成功] 已提交并推送")


def branch_delete(project_path: str, branch_name: str):
    DELETABLE_PREFIXES = ["feature/", "hotfix/"]

    project_path = validate_project_path(project_path)

    if not any(branch_name.startswith(prefix) for prefix in DELETABLE_PREFIXES):
        print(f"[错误] 只允许删除 feature/* 或 hotfix/* 分支")
        sys.exit(1)

    check_uncommitted_changes(project_path)

    print(f"即将删除: {branch_name}")
    confirm = input("确认删除？:").strip().lower()
    if confirm != "y":
        print("[取消] 操作已取消")
        sys.exit(0)

    ok, output = run_git(project_path, "branch", "-D", branch_name)
    if not ok:
        print(f"[错误] 删除本地分支失败: {output}")
        sys.exit(1)

    run_git(project_path, "push", "origin", "--delete", branch_name)
    print(f"[成功] 已删除: {branch_name}")


def branch_clean(project_path: str, stale_days: int = 60):
    PROTECTED_BRANCHES = ["master", "main", "develop", "staging", "production"]
    CLEANABLE_PREFIXES = ["feature/", "hotfix/"]

    project_path = validate_project_path(project_path)
    check_uncommitted_changes(project_path)

    fmt = "%(refname:short)|%(committerdate:iso8601)|%(authorname)"
    ok, output = run_git(project_path, "for-each-ref", "--sort=-committerdate", f"--format={fmt}", "refs/heads/")

    if not ok or not output.strip():
        print("[信息] 未找到任何本地分支")
        sys.exit(0)

    branches = []
    for line in output.strip().splitlines():
        parts = line.split("|", 2)
        if len(parts) != 3:
            continue
        name, date_str, author = parts
        if name in PROTECTED_BRANCHES:
            continue
        if not any(name.startswith(prefix) for prefix in CLEANABLE_PREFIXES):
            continue
        try:
            last_commit = datetime.fromisoformat(date_str.strip())
        except (ValueError, IndexError):
            continue
        branches.append({"name": name, "last_commit": last_commit, "author": author.strip()})

    now = datetime.now().astimezone()
    cutoff = now - timedelta(days=stale_days)
    stale_branches = [b for b in branches if b["last_commit"] < cutoff]

    if not stale_branches:
        print(f"[信息] 没有超过 {stale_days} 天无提交的过期分支")
        sys.exit(0)

    print(f"过期分支（{stale_days}天无提交）:")
    for b in stale_branches:
        days_ago = (now - b["last_commit"]).days
        print(f"  - {b['name']} ({days_ago}天前)")

    confirm = input(f"确认删除以上 {len(stale_branches)} 个过期分支？:").strip().lower()
    if confirm != "y":
        print("[取消] 操作已取消")
        sys.exit(0)

    run_git(project_path, "checkout", "develop")
    deleted = 0
    for b in stale_branches:
        run_git(project_path, "branch", "-D", b["name"])
        run_git(project_path, "push", "origin", "--delete", b["name"])
        deleted += 1

    print(f"[成功] 已删除 {deleted} 个过期分支")


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    command = sys.argv[1]
    args = sys.argv[2:]

    commands = {
        "feature_start": (feature_start, 2, "python git_flow.py feature_start <项目路径> <分支名>"),
        "feature_finish": (feature_finish, 2, "python git_flow.py feature_finish <项目路径> <分支名>"),
        "hotfix_start": (hotfix_start, 2, "python git_flow.py hotfix_start <项目路径> <分支名>"),
        "hotfix_finish": (hotfix_finish, 2, "python git_flow.py hotfix_finish <项目路径> <分支名>"),
        "deploy": (deploy, 2, "python git_flow.py deploy <项目路径> <环境> [提交信息]"),
        "commit": (commit, 2, "python git_flow.py commit <项目路径> <提交信息>"),
        "branch_delete": (branch_delete, 2, "python git_flow.py branch_delete <项目路径> <分支名>"),
        "branch_clean": (branch_clean, 1, "python git_flow.py branch_clean <项目路径> [过期天数]"),
    }

    if command not in commands:
        print(f"[错误] 未知命令: {command}")
        print(f"可用命令: {', '.join(commands.keys())}")
        sys.exit(1)

    func, min_args, usage = commands[command]

    if len(args) < min_args:
        print(f"[错误] 参数不足")
        print(f"用法: {usage}")
        sys.exit(1)

    func(*args)


if __name__ == "__main__":
    main()
