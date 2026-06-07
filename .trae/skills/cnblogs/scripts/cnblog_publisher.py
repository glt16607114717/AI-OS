"""
博客园 MetaWeblog API 发布工具
支持发布、编辑、删除、查询博客文章
基于 XML-RPC 协议调用博客园 MetaWeblog API
本地文章存档目录: D:/wwwroot/ai/ai_cache/cnblogs/
"""
import sys
import re
import json
import ssl
import argparse
from pathlib import Path
from datetime import datetime
import xmlrpc.client

BASE_DIR = Path(__file__).resolve().parent
CONFIG_FILE = BASE_DIR / "cnblogs_config.json"
ARTICLES_DIR = Path("D:/wwwroot/ai/ai_cache/cnblogs")

PERSONAL_CATEGORY_PREFIX = "[随笔分类]"
SPECIAL_CATEGORIES = {"[Markdown]", "[发布至博客园首页]", "[发布为日记]", "[发布为文章]", "[发布为新闻]"}


def sanitize_filename(name: str) -> str:
    return re.sub(r'[\\/:*?"<>|]', '_', name).strip()


def article_path(post_id: str, title: str) -> Path:
    ARTICLES_DIR.mkdir(parents=True, exist_ok=True)
    return ARTICLES_DIR / f"{post_id}-{sanitize_filename(title)}.md"


def save_local(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def load_config() -> dict:
    if not CONFIG_FILE.exists():
        print(json.dumps({
            "success": False,
            "error": f"配置文件不存在: {CONFIG_FILE}",
            "hint": "请在 cnblogs_config.json 中填写 blog_name、username、access_token"
        }, ensure_ascii=False))
        sys.exit(1)
    with open(CONFIG_FILE, "r", encoding="utf-8") as f:
        return json.load(f)


def create_proxy(endpoint: str) -> xmlrpc.client.ServerProxy:
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    return xmlrpc.client.ServerProxy(endpoint, context=ctx)


def get_blog_info(config: dict) -> dict:
    proxy = create_proxy(config["endpoint"])
    try:
        result = proxy.blogger.getUsersBlogs("", config["username"], config["access_token"])
        if isinstance(result, list) and len(result) > 0:
            info = result[0]
            return {
                "success": True,
                "blog_id": info.get("blogid", ""),
                "blog_name": info.get("blogName", ""),
                "blog_url": info.get("url", "")
            }
        return {"success": False, "error": "未获取到博客信息，请检查配置"}
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"连接失败: {str(e)}"}


def get_categories(config: dict) -> dict:
    proxy = create_proxy(config["endpoint"])
    blog_id = config.get("blog_name", config["username"])
    try:
        cats = proxy.metaWeblog.getCategories(blog_id, config["username"], config["access_token"])
        personal = []
        for c in cats:
            title = c.get("title", "")
            if title.startswith(PERSONAL_CATEGORY_PREFIX):
                personal.append({
                    "full_name": title,
                    "name": title.replace(PERSONAL_CATEGORY_PREFIX, ""),
                })
        return {
            "success": True,
            "count": len(personal),
            "categories": personal
        }
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"获取分类失败: {str(e)}"}


def build_categories_list(category: str = None, tags: list = None,
                          is_markdown: bool = True, publish_to_home: bool = False) -> list:
    categories = []
    if is_markdown:
        categories.append("[Markdown]")
    if publish_to_home:
        categories.append("[发布至博客园首页]")
    if category:
        if category.startswith("[随笔分类]"):
            categories.append(category)
        else:
            categories.append(f"[随笔分类]{category}")
    if tags:
        for t in tags:
            t = t.strip()
            if t and t not in categories:
                categories.append(t)
    return categories


def parse_raw_categories(raw_cats: list) -> tuple:
    category = ""
    tags = []
    for c in raw_cats:
        if c.startswith(PERSONAL_CATEGORY_PREFIX):
            category = c.replace(PERSONAL_CATEGORY_PREFIX, "")
        elif c not in SPECIAL_CATEGORIES:
            tags.append(c)
    return category, tags


def publish_post(config: dict, title: str, content: str, category: str = None,
                 tags: list = None, publish: bool = True, is_markdown: bool = True,
                 publish_to_home: bool = False) -> dict:
    proxy = create_proxy(config["endpoint"])
    categories = build_categories_list(
        category=category, tags=tags,
        is_markdown=is_markdown, publish_to_home=publish_to_home
    )
    post_struct = {
        "title": title,
        "description": content,
        "categories": categories,
    }
    blog_id = config.get("blog_name", config["username"])
    try:
        post_id = proxy.metaWeblog.newPost(
            blog_id, config["username"], config["access_token"], post_struct, publish
        )
        post_id = str(post_id)
        local_file = article_path(post_id, title)
        save_local(local_file, content)
        return {
            "success": True,
            "post_id": post_id,
            "title": title,
            "category": category or "未分类",
            "tags": tags or [],
            "status": "已发布" if publish else "草稿",
            "url": f"https://www.cnblogs.com/{config['blog_name']}/p/{post_id}.html",
            "local_file": str(local_file)
        }
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"发布失败: {str(e)}"}


def edit_post(config: dict, post_id: str, title: str = None, content: str = None,
              category: str = None, tags: list = None, publish: bool = True,
              is_markdown: bool = True, publish_to_home: bool = False) -> dict:
    proxy = create_proxy(config["endpoint"])
    post_struct = {}
    if title:
        post_struct["title"] = title
    if content:
        post_struct["description"] = content
    if category is not None or tags is not None:
        post_struct["categories"] = build_categories_list(
            category=category, tags=tags,
            is_markdown=is_markdown, publish_to_home=publish_to_home
        )
    try:
        result = proxy.metaWeblog.editPost(
            post_id, config["username"], config["access_token"], post_struct, publish
        )
        if result and content:
            final_title = title or post_id
            local_file = article_path(post_id, final_title)
            save_local(local_file, content)
        return {
            "success": bool(result),
            "post_id": post_id,
            "title": title or "(未修改标题)",
            "status": "已更新" if result else "更新失败"
        }
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"编辑失败: {str(e)}"}


def get_post(config: dict, post_id: str) -> dict:
    proxy = create_proxy(config["endpoint"])
    try:
        post = proxy.metaWeblog.getPost(post_id, config["username"], config["access_token"])
        raw_cats = post.get("categories", [])
        category, tags = parse_raw_categories(raw_cats)
        title = post.get("title", "")
        description = post.get("description", "")
        local_file = article_path(post_id, title)
        save_local(local_file, description)
        return {
            "success": True,
            "post_id": str(post.get("postid", post_id)),
            "title": title,
            "description": description,
            "category": category,
            "tags": tags,
            "date_created": str(post.get("dateCreated", "")),
            "link": post.get("link", ""),
            "local_file": str(local_file)
        }
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"获取失败: {str(e)}"}


def get_recent_posts(config: dict, count: int = 10) -> dict:
    proxy = create_proxy(config["endpoint"])
    blog_id = config.get("blog_name", config["username"])
    try:
        posts = proxy.metaWeblog.getRecentPosts(
            blog_id, config["username"], config["access_token"], count
        )
        result = []
        for p in posts:
            raw_cats = p.get("categories", [])
            category, tags = parse_raw_categories(raw_cats)
            title = p.get("title", "")
            post_id = str(p.get("postid", ""))
            local_file = article_path(post_id, title)
            result.append({
                "post_id": post_id,
                "title": title,
                "date_created": str(p.get("dateCreated", "")),
                "link": p.get("link", ""),
                "category": category,
                "tags": tags,
                "local_file": str(local_file)
            })
        return {"success": True, "count": len(result), "posts": result}
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"获取失败: {str(e)}"}


def delete_post(config: dict, post_id: str) -> dict:
    proxy = create_proxy(config["endpoint"])
    try:
        result = proxy.blogger.deletePost("", post_id, config["username"], config["access_token"], True)
        return {
            "success": bool(result),
            "post_id": post_id,
            "status": "已删除" if result else "删除失败"
        }
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"删除失败: {str(e)}"}


def sync_posts(config: dict, count: int = 50) -> dict:
    proxy = create_proxy(config["endpoint"])
    blog_id = config.get("blog_name", config["username"])
    try:
        posts = proxy.metaWeblog.getRecentPosts(
            blog_id, config["username"], config["access_token"], count
        )
        synced = []
        for p in posts:
            title = p.get("title", "")
            post_id = str(p.get("postid", ""))
            description = p.get("description", "")
            local_file = article_path(post_id, title)
            if not local_file.exists():
                save_local(local_file, description)
                synced.append({
                    "post_id": post_id,
                    "title": title,
                    "local_file": str(local_file)
                })
        return {
            "success": True,
            "total": len(posts),
            "synced": len(synced),
            "already_exists": len(posts) - len(synced),
            "articles": synced
        }
    except xmlrpc.client.Fault as e:
        return {"success": False, "error": f"API错误: {e.faultString} (code: {e.faultCode})"}
    except Exception as e:
        return {"success": False, "error": f"同步失败: {str(e)}"}


def read_content_from_file(file_path: str) -> str:
    p = Path(file_path)
    if not p.exists():
        return ""
    return p.read_text(encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description="博客园 MetaWeblog 发布工具")
    subparsers = parser.add_subparsers(dest="action", help="操作类型")

    info_parser = subparsers.add_parser("info", help="获取博客信息")
    info_parser.add_argument("--verify", action="store_true", help="验证配置是否正确")

    cat_parser = subparsers.add_parser("categories", help="获取随笔分类列表")

    pub_parser = subparsers.add_parser("publish", help="发布新文章（仅支持 --file）")
    pub_parser.add_argument("--title", required=True, help="文章标题")
    pub_parser.add_argument("--file", required=True, help="Markdown文件路径")
    pub_parser.add_argument("--category", help="随笔分类名称（如：AI、php、mysql）")
    pub_parser.add_argument("--tags", help="标签，逗号分隔")
    pub_parser.add_argument("--draft", action="store_true", help="保存为草稿（默认直接发布）")
    pub_parser.add_argument("--no-markdown", action="store_true", help="内容不是Markdown格式")
    pub_parser.add_argument("--publish-to-home", action="store_true", help="发布至博客园首页")

    edit_parser = subparsers.add_parser("edit", help="编辑已有文章")
    edit_parser.add_argument("--post-id", required=True, help="文章ID")
    edit_parser.add_argument("--title", help="新标题")
    edit_parser.add_argument("--file", help="Markdown文件路径")
    edit_parser.add_argument("--category", help="随笔分类名称")
    edit_parser.add_argument("--tags", help="新标签，逗号分隔")

    get_parser = subparsers.add_parser("get", help="获取文章详情（自动存档本地）")
    get_parser.add_argument("--post-id", required=True, help="文章ID")

    list_parser = subparsers.add_parser("list", help="获取最近文章列表")
    list_parser.add_argument("--count", type=int, default=10, help="获取数量（默认10）")

    sync_parser = subparsers.add_parser("sync", help="同步远程文章到本地存档")
    sync_parser.add_argument("--count", type=int, default=50, help="同步数量（默认50）")

    del_parser = subparsers.add_parser("delete", help="删除文章")
    del_parser.add_argument("--post-id", required=True, help="文章ID")

    args = parser.parse_args()
    config = load_config()

    if not config.get("access_token"):
        print(json.dumps({
            "success": False,
            "error": "access_token 未配置",
            "hint": "请在博客园后台「管理 → 设置 → 其他设置 → MetaWeblog访问令牌」获取令牌，填入 cnblogs_config.json"
        }, ensure_ascii=False))
        sys.exit(1)

    if args.action == "info":
        result = get_blog_info(config)
        if args.verify and result.get("success"):
            result["message"] = "配置验证通过，博客连接正常"
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "categories":
        result = get_categories(config)
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "publish":
        content = read_content_from_file(args.file)
        if not content:
            print(json.dumps({"success": False, "error": f"文件不存在或为空: {args.file}"}, ensure_ascii=False))
            sys.exit(1)
        tags = [t.strip() for t in args.tags.split(",")] if args.tags else []
        result = publish_post(
            config, args.title, content,
            category=args.category, tags=tags,
            publish=not args.draft,
            is_markdown=not args.no_markdown,
            publish_to_home=args.publish_to_home
        )
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "edit":
        content = None
        if args.file:
            content = read_content_from_file(args.file)
            if not content:
                print(json.dumps({"success": False, "error": f"文件不存在或为空: {args.file}"}, ensure_ascii=False))
                sys.exit(1)
        tags = [t.strip() for t in args.tags.split(",")] if args.tags else None
        result = edit_post(
            config, args.post_id, title=args.title, content=content,
            category=args.category, tags=tags
        )
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "get":
        result = get_post(config, args.post_id)
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "list":
        result = get_recent_posts(config, args.count)
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "sync":
        result = sync_posts(config, args.count)
        print(json.dumps(result, ensure_ascii=False))

    elif args.action == "delete":
        result = delete_post(config, args.post_id)
        print(json.dumps(result, ensure_ascii=False))

    else:
        parser.print_help()


if __name__ == "__main__":
    main()
