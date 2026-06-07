import json
import sys
import os
import time
import random
import requests
from pathlib import Path

CONFIG_PATH = Path(__file__).parent / "apipost_config.json"
TEMP_DIR = Path(__file__).parent / "temp"


def read_input() -> str:
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                return f.read().strip()
    return ""


def load_config():
    with open(CONFIG_PATH, "r", encoding="utf-8") as f:
        return json.load(f)


def generate_id():
    return hex(int(time.time() * 1000) + random.randint(0, 9999))[2:]


def get_headers(token):
    return {
        "Api-Token": token,
        "Content-Type": "application/json"
    }


def api_call(config, method, endpoint, data=None, params=None):
    url = f"{config['host']}{endpoint}"
    headers = get_headers(config["token"])
    try:
        if method == "GET":
            resp = requests.get(url, headers=headers, params=params, timeout=30)
        else:
            resp = requests.post(url, headers=headers, json=data, timeout=30)
        result = resp.json()
        if result.get("code") != 0:
            return {"success": False, "error": result.get("msg", "unknown error"), "code": result.get("code")}
        return {"success": True, "data": result.get("data")}
    except Exception as e:
        return {"success": False, "error": str(e)}


def build_param(key, description="", field_type="string", required=False, value=""):
    return {
        "param_id": generate_id(),
        "key": key,
        "value": str(value) if value is not None else "",
        "description": description,
        "field_type": field_type,
        "is_checked": 1,
        "not_null": 1 if required else -1,
        "schema": {"type": field_type}
    }


def build_api_template(method, url, name, parent_id="0", description="",
                        query_params=None, body_params=None, headers=None,
                        response_raw="", response_fields=None):
    tpl = {
        "target_id": generate_id(),
        "target_type": "api",
        "parent_id": parent_id,
        "name": name,
        "method": method.upper(),
        "url": url,
        "protocol": "http/1.1",
        "description": description or f"{name} - {method} {url}",
        "version": 3,
        "mark_id": 1,
        "is_force": -1,
        "is_deleted": -1,
        "is_conflicted": -1,
        "sort": 0,
        "status": 1,
        "request": {
            "auth": {"type": "inherit"},
            "pre_tasks": [],
            "post_tasks": [],
            "header": {"parameter": [build_param(k, d) for k, d in (headers or [])]},
            "query": {
                "query_add_equal": 1,
                "parameter": [build_param(k, d, ft, req) for k, d, ft, req in (query_params or [])]
            },
            "body": {
                "mode": "json" if body_params else "none",
                "parameter": [],
                "raw": json.dumps(body_params, ensure_ascii=False, indent=4) if body_params else "",
                "raw_parameter": [],
                "raw_schema": {},
                "binary": None
            },
            "cookie": {"cookie_encode": 1, "parameter": []},
            "restful": {"parameter": []}
        },
        "response": {
            "example": [{
                "example_id": "1",
                "raw": response_raw or '{"code": 200, "msg": "success", "data": {}}',
                "raw_parameter": [build_param(k, d, ft) for k, d, ft in (response_fields or [])],
                "headers": [],
                "expect": {
                    "code": "200",
                    "content_type": "application/json",
                    "is_default": 1,
                    "mock": "",
                    "name": "ok",
                    "schema": {"type": "object", "properties": {}},
                    "verify_type": "schema",
                    "sleep": 0
                }
            }],
            "is_check_result": 1
        },
        "tags": []
    }
    return tpl


def build_folder_template(name, parent_id="0", description=""):
    return {
        "target_id": generate_id(),
        "target_type": "folder",
        "parent_id": parent_id,
        "name": name,
        "description": description,
        "sort": 0,
        "version": 0,
        "status": 1,
        "is_force": -1,
        "is_deleted": -1,
        "is_conflicted": -1,
        "request": {
            "auth": {"type": "inherit"},
            "pre_tasks": [],
            "post_tasks": [],
            "header": {"parameter": []},
            "query": {"parameter": []},
            "body": {"parameter": [], "raw": "", "raw_parameter": []},
            "cookie": {"parameter": []},
            "restful": {"parameter": []}
        },
        "response": {"example": []},
        "tags": []
    }


def cmd_list_teams(config, args):
    return api_call(config, "GET", "/open/team/list")


def cmd_list_projects(config, args):
    params = {}
    if args.get("team_id"):
        params["team_id"] = args["team_id"]
    return api_call(config, "GET", "/open/project/list", params=params)


def cmd_list_apis(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    return api_call(config, "GET", "/open/apis/list", params={"project_id": project_id})


def cmd_get_api_detail(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    target_id = args.get("target_id")
    if not target_id:
        return {"success": False, "error": "target_id required"}
    return api_call(config, "POST", "/open/apis/details", data={
        "project_id": project_id,
        "target_id": target_id
    })


def cmd_create_folder(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    name = args.get("name")
    parent_id = args.get("parent_id", "0")
    description = args.get("description", "")

    if not name:
        return {"success": False, "error": "name required"}

    tpl = build_folder_template(name, parent_id, description)
    tpl["project_id"] = project_id
    result = api_call(config, "POST", "/open/apis/create", data=tpl)
    if result["success"]:
        result["target_id"] = tpl["target_id"]
        result["parent_id"] = parent_id
    return result


def cmd_create_api(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    name = args.get("name")
    method = args.get("method", "GET")
    url = args.get("url", "")
    parent_id = args.get("parent_id", "0")
    description = args.get("description", "")

    if not name:
        return {"success": False, "error": "name required"}

    query_params = args.get("query_params", [])
    body_params = args.get("body_params")
    headers = args.get("headers", [])
    response_raw = args.get("response_raw", "")
    response_fields = args.get("response_fields", [])

    tpl = build_api_template(
        method, url, name, parent_id, description,
        query_params=query_params,
        body_params=body_params,
        headers=headers,
        response_raw=response_raw,
        response_fields=response_fields
    )
    tpl["project_id"] = project_id

    result = api_call(config, "POST", "/open/apis/create", data=tpl)
    if result["success"]:
        result["target_id"] = tpl["target_id"]
        result["parent_id"] = parent_id
    return result


def cmd_update_api(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    target_id = args.get("target_id")
    if not target_id:
        return {"success": False, "error": "target_id required"}

    detail = api_call(config, "POST", "/open/apis/details", data={
        "project_id": project_id,
        "target_id": target_id
    })
    if not detail["success"]:
        return detail

    original = detail["data"]
    original_version = int(original.get("version", 0)) + 1

    update_fields = args.get("update_fields", {})

    tpl = build_api_template(
        update_fields.get("method", original.get("method", "GET")),
        update_fields.get("url", original.get("url", "")),
        update_fields.get("name", original.get("name", "")),
        original.get("parent_id", "0"),
        update_fields.get("description", original.get("description", "")),
        query_params=update_fields.get("query_params"),
        body_params=update_fields.get("body_params"),
        response_raw=update_fields.get("response_raw"),
        response_fields=update_fields.get("response_fields")
    )
    tpl["target_id"] = target_id
    tpl["project_id"] = project_id
    tpl["parent_id"] = original.get("parent_id", "0")
    tpl["version"] = original_version

    return api_call(config, "POST", "/open/apis/update", data=tpl)


def cmd_move_api(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    target_id = args.get("target_id")
    new_parent_id = args.get("parent_id")
    if not target_id or not new_parent_id:
        return {"success": False, "error": "target_id and parent_id required"}

    detail = api_call(config, "POST", "/open/apis/details", data={
        "project_id": project_id,
        "target_id": target_id
    })
    if not detail["success"]:
        return detail

    original = detail["data"]
    original["parent_id"] = new_parent_id
    original["version"] = int(original.get("version", 0)) + 1
    original["project_id"] = project_id

    return api_call(config, "POST", "/open/apis/update", data=original)


def cmd_delete_apis(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    target_ids = args.get("target_ids", [])
    if not target_ids:
        return {"success": False, "error": "target_ids required"}

    return api_call(config, "POST", "/open/apis/delete", data={
        "project_id": project_id,
        "target_ids": target_ids
    })


def cmd_search(config, args):
    project_id = args.get("project_id", config.get("project_id"))
    keyword = args.get("keyword", "")
    parent_id = args.get("parent_id")
    target_type = args.get("target_type", "all")

    result = api_call(config, "GET", "/open/apis/list", params={"project_id": project_id})
    if not result["success"]:
        return result

    items = result["data"].get("list", [])

    if parent_id is not None:
        items = [i for i in items if i.get("parent_id") == parent_id]

    if target_type != "all":
        items = [i for i in items if i.get("target_type") == target_type]

    if keyword:
        kw = keyword.lower()
        items = [i for i in items if kw in i.get("name", "").lower() or kw in i.get("url", "").lower()]

    return {"success": True, "data": {"list": items, "total": len(items)}}


COMMANDS = {
    "list_teams": cmd_list_teams,
    "list_projects": cmd_list_projects,
    "list_apis": cmd_list_apis,
    "get_detail": cmd_get_api_detail,
    "search": cmd_search,
    "create_folder": cmd_create_folder,
    "create_api": cmd_create_api,
    "update_api": cmd_update_api,
    "move_api": cmd_move_api,
    "delete_apis": cmd_delete_apis,
}


def main():
    try:
        raw = read_input()
        if not raw.strip():
            print(json.dumps({"success": False, "error": "no input"}, ensure_ascii=False))
            return

        args = json.loads(raw)
        command = args.get("command")

        if not command:
            print(json.dumps({"success": False, "error": f"command required, available: {', '.join(COMMANDS.keys())}"}, ensure_ascii=False))
            return

        if command not in COMMANDS:
            print(json.dumps({"success": False, "error": f"unknown command: {command}, available: {', '.join(COMMANDS.keys())}"}, ensure_ascii=False))
            return

        config = load_config()
        if not config.get("token"):
            print(json.dumps({"success": False, "error": "token not configured in apipost_config.json"}, ensure_ascii=False))
            return

        result = COMMANDS[command](config, args)
        print(json.dumps(result, ensure_ascii=False, indent=2))

    except json.JSONDecodeError as e:
        print(json.dumps({"success": False, "error": f"JSON parse error: {e}"}, ensure_ascii=False))
    except FileNotFoundError:
        print(json.dumps({"success": False, "error": f"config not found: {CONFIG_PATH}"}, ensure_ascii=False))
    except Exception as e:
        print(json.dumps({"success": False, "error": str(e)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
