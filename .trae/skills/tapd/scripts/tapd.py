import sys
import json
import os
from pathlib import Path

try:
    import markdown as md_lib
except ImportError:
    md_lib = None

sys.path.insert(0, str(Path(__file__).parent))
from tapd_client import TAPDClient

WORKSPACE_ID = 66680814


def _md_to_html(text: str) -> str:
    if not md_lib or not text:
        return text
    html = md_lib.markdown(text, extensions=["extra", "codehilite", "toc"])
    if any(tag in html for tag in ("<p>", "<h", "<ul>", "<ol>")):
        return html
    return text


def _merge_params(args: dict) -> dict:
    workspace_id = args.get("workspace_id", WORKSPACE_ID)
    options = args.get("options") or {}
    merged = {"workspace_id": workspace_id}
    merged.update(options)
    return merged


def _err(msg: str) -> str:
    return json.dumps({"status": 0, "info": msg, "data": None}, ensure_ascii=False, indent=2)


def _out(data) -> str:
    return json.dumps(data, ensure_ascii=False, indent=2)


# ── workspace ──────────────────────────────────────────────

def tool_get_user_projects(client: TAPDClient, args: dict) -> str:
    nick = args.get("nick") or ""
    params = {}
    if nick:
        params["nick"] = nick
    elif client.nick:
        params["nick"] = client.nick
    else:
        return _err("用户昵称不能为空，请提供 nick 参数")
    ret = client.get_user_participant_projects(params)
    if ret.get("data") and isinstance(ret["data"], list):
        ret["data"] = [
            p for p in ret["data"]
            if not (isinstance(p, dict) and p.get("Workspace", {}).get("category") == "organization")
        ]
    return _out(ret)


def tool_get_workspace_info(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_workspace_info(params))


# ── stories / tasks (smart) ────────────────────────────────

def tool_get_stories_or_tasks(client: TAPDClient, args: dict) -> str:
    workspace_id = args.get("workspace_id", WORKSPACE_ID)
    if not workspace_id:
        return tool_get_user_projects(client, args)
    options = args.get("options") or {}
    if "entity_type" not in options:
        options["entity_type"] = "stories"

    if "category_name" in options:
        cat_result = client.get_story_categories(
            {"workspace_id": workspace_id, "name": options["category_name"]}
        )
        cats = (cat_result.get("data") or []) if isinstance(cat_result, dict) else []
        if not cats:
            return _err(f"未找到需求分类: {options['category_name']}")
        if len(cats) == 1:
            options["category_id"] = cats[0]["Category"]["id"]
            del options["category_name"]
        else:
            names = [f"{c['Category']['id']}:{c['Category']['name']}" for c in cats]
            return _err(f"有多个分类，请选择: {', '.join(names)}")

    if "iteration_name" in options:
        it_result = client.get_iterations(
            {"workspace_id": workspace_id, "name": options["iteration_name"]}
        )
        its = (it_result.get("data") or []) if isinstance(it_result, dict) else []
        if not its:
            return _err(f"未找到迭代: {options['iteration_name']}")
        if len(its) == 1:
            options["iteration_id"] = its[0]["Iteration"]["id"]
            del options["iteration_name"]
        else:
            names = [f"{i['Iteration']['id']}:{i['Iteration']['name']}" for i in its]
            return _err(f"有多个迭代，请选择: {', '.join(names)}")

    if "workitem_type_name" in options:
        wt_result = client.get_workitem_types(
            {"workspace_id": workspace_id, "name": options["workitem_type_name"]}
        )
        wts = (wt_result.get("data") or []) if isinstance(wt_result, dict) else []
        if not wts:
            return _err(f"未找到需求类别: {options['workitem_type_name']}")
        if len(wts) == 1:
            options["workitem_type_id"] = wts[0]["WorkitemType"]["id"]
            del options["workitem_type_name"]
        else:
            names = [f"{w['WorkitemType']['id']}:{w['WorkitemType']['name']}" for w in wts]
            return _err(f"有多个需求类别，请选择: {', '.join(names)}")

    merged = {"workspace_id": workspace_id}
    merged.update(options)

    ret = client.get_stories(merged)
    count_ret = client.get_story_count(merged)

    fields_param = options.get("fields")
    if isinstance(ret, dict) and "data" in ret and isinstance(ret["data"], list):
        ret["data"] = client.filter_fields(ret["data"], fields_param)

    entity_type = options["entity_type"]
    url_template = client.get_story_or_task_url_template(workspace_id, entity_type)

    return _out({
        "url_template": url_template,
        "data": ret.get("data") if isinstance(ret, dict) else ret,
        "count": count_ret,
    })


def tool_update_story_or_task(client: TAPDClient, args: dict) -> str:
    workspace_id = args.get("workspace_id", WORKSPACE_ID)
    if not workspace_id:
        return tool_get_user_projects(client, args)
    options = args.get("options") or {}
    if "entity_type" not in options:
        options["entity_type"] = "stories"
    merged = {"workspace_id": workspace_id}
    merged.update(options)
    if "description" in merged:
        merged["description"] = _md_to_html(merged["description"])
    result = client.create_or_update_story(merged)
    if isinstance(result, dict) and "data" in result:
        result["data"] = client.filter_fields_for_create_or_update(result["data"])
    entity_type = options["entity_type"]
    url_template = client.get_story_or_task_url_template(workspace_id, entity_type)
    return _out({"url_template": url_template, "data": result})


def tool_get_story_count(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    if "entity_type" not in params:
        params["entity_type"] = "stories"
    return _out(client.get_story_count(params))


def tool_get_stories_custom_fields(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_stories_custom_fields_settings(params))


def tool_get_stories_fields_label(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_stories_fields_label(params))


def tool_get_stories_fields_info(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_stories_fields_info(params))


def tool_get_related_bugs(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_related_bugs(params))


# ── raw stories / tasks ────────────────────────────────────

def tool_get_stories(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_stories(params))


def tool_create_or_update_story(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    if "description" in params:
        params["description"] = _md_to_html(params["description"])
    return _out(client.create_or_update_story(params))


def tool_get_tasks(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    params["entity_type"] = "tasks"
    return _out(client.get_stories(params))


def tool_create_or_update_task(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    params["entity_type"] = "tasks"
    if "description" in params:
        params["description"] = _md_to_html(params["description"])
    return _out(client.create_or_update_story(params))


def tool_get_task_count(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    params["entity_type"] = "tasks"
    return _out(client.get_story_count(params))


# ── bugs ───────────────────────────────────────────────────

def tool_get_bugs(client: TAPDClient, args: dict) -> str:
    workspace_id = args.get("workspace_id", WORKSPACE_ID)
    if not workspace_id:
        return tool_get_user_projects(client, args)
    merged = _merge_params(args)
    ret = client.get_bugs(merged)
    count_ret = client.get_bug_count(merged)
    fields_param = (args.get("options") or {}).get("fields")
    if isinstance(ret, dict) and "data" in ret and isinstance(ret["data"], list):
        ret["data"] = client.filter_fields(ret["data"], fields_param)
    return _out({
        "base_url": client.tapd_base_url,
        "data": ret.get("data") if isinstance(ret, dict) else ret,
        "count": count_ret,
    })


def tool_get_bug_count(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_bug_count(params))


def tool_update_bug(client: TAPDClient, args: dict) -> str:
    workspace_id = args.get("workspace_id", WORKSPACE_ID)
    if not workspace_id:
        return tool_get_user_projects(client, args)
    merged = _merge_params(args)
    if "description" in merged:
        merged["description"] = _md_to_html(merged["description"])
    if "lastmodify" not in merged and client.nick:
        merged["lastmodify"] = client.nick
    result = client.create_or_update_bug(merged)
    if isinstance(result, dict) and "data" in result:
        result["data"] = client.filter_fields_for_create_or_update(result["data"])
    return _out({"base_url": client.tapd_base_url, "data": result})


def tool_get_bug_custom_fields(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_bug_custom_fields(params))


# ── comments ───────────────────────────────────────────────

def tool_get_comments(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_comments(params))


def tool_create_comment(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    if "description" in params:
        params["description"] = _md_to_html(params["description"])
    return _out(client.create_comments(params))


# ── files / attachments ────────────────────────────────────

def tool_get_image(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    if "image_path" not in params:
        return _err("image_path 为必填参数")
    return _out(client.get_image(params))


def tool_get_attachments(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    if "entry_id" not in params:
        return _err("entry_id 为必填参数")
    if "type" not in params:
        return _err("type 为必填参数 (story/bug)")
    return _out(client.get_attachments(params))


def tool_get_attachment_download_url(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_attachment_download_url(params))


# ── iterations ─────────────────────────────────────────────

def tool_get_iterations(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_iterations(params))


def tool_create_or_update_iteration(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.create_or_update_iteration(params))


# ── workflows ──────────────────────────────────────────────

def tool_get_workflows_all_transitions(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_workflows_all_transitions(params))


def tool_get_workflows_status_map(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_workflows_status_map(params))


def tool_get_workflows_last_steps(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_workflows_last_steps(params))


# ── workitem types / categories ────────────────────────────

def tool_get_workitem_types(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_workitem_types(params))


def tool_get_entity_custom_fields(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_entity_custom_fields(params))


def tool_get_story_categories(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_story_categories(params))


# ── test cases ─────────────────────────────────────────────

def tool_get_tcases(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_tcases(params))


def tool_create_tcases(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.create_tcases(params))


def tool_create_tcases_batch(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.create_tcases_batch(params))


def tool_get_tcases_count(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_tcases_count(params))


def tool_get_tcases_custom_fields(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_tcases_custom_fields(params))


# ── wiki ───────────────────────────────────────────────────

def tool_get_wikis(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_wikis(params))


def tool_create_wiki(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.create_wiki(params))


def tool_get_wiki_count(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_wiki_count(params))


# ── user / todo ────────────────────────────────────────────

def tool_get_user_info(client: TAPDClient, args: dict) -> str:
    nick = client.get_user_info()
    return _out({"status": 1, "data": {"nick": nick}})


def tool_get_todo(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_todo(params))


def tool_get_user_story_todo(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_user_story_todo(params))


def tool_get_user_bug_todo(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_user_bug_todo(params))


def tool_get_user_task_todo(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_user_task_todo(params))


# ── timesheets ─────────────────────────────────────────────

def tool_update_timesheets(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.update_timesheets(params))


def tool_get_timesheets(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_timesheets(params))


# ── scm / release / relations ──────────────────────────────

def tool_get_scm_copy_keywords(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_scm_copy_keywords(params))


def tool_get_release_info(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.get_release_info(params))


def tool_add_entity_relations(client: TAPDClient, args: dict) -> str:
    params = _merge_params(args)
    return _out(client.add_entity_relations(params))


# ── qiwei message ──────────────────────────────────────────

def tool_send_qiwei_message(client: TAPDClient, args: dict) -> str:
    msg = args.get("msg", "")
    if not msg:
        return _err("msg 为必填参数")
    return client.send_message({"msg": msg})


# ── tool registry ──────────────────────────────────────────

TOOLS: dict = {
    # workspace
    "get_user_projects": tool_get_user_projects,
    "get_workspace_info": tool_get_workspace_info,
    # stories (smart)
    "get_stories_or_tasks": tool_get_stories_or_tasks,
    "update_story_or_task": tool_update_story_or_task,
    "get_story_count": tool_get_story_count,
    "get_stories_custom_fields": tool_get_stories_custom_fields,
    "get_stories_fields_label": tool_get_stories_fields_label,
    "get_stories_fields_info": tool_get_stories_fields_info,
    "get_related_bugs": tool_get_related_bugs,
    # stories (raw)
    "get_stories": tool_get_stories,
    "create_or_update_story": tool_create_or_update_story,
    # tasks (raw)
    "get_tasks": tool_get_tasks,
    "create_or_update_task": tool_create_or_update_task,
    "get_task_count": tool_get_task_count,
    # bugs
    "get_bugs": tool_get_bugs,
    "get_bug_count": tool_get_bug_count,
    "update_bug": tool_update_bug,
    "get_bug_custom_fields": tool_get_bug_custom_fields,
    # comments
    "get_comments": tool_get_comments,
    "create_comment": tool_create_comment,
    # files / attachments
    "get_image": tool_get_image,
    "get_attachments": tool_get_attachments,
    "get_attachment_download_url": tool_get_attachment_download_url,
    # iterations
    "get_iterations": tool_get_iterations,
    "create_or_update_iteration": tool_create_or_update_iteration,
    # workflows
    "get_workflows_all_transitions": tool_get_workflows_all_transitions,
    "get_workflows_status_map": tool_get_workflows_status_map,
    "get_workflows_last_steps": tool_get_workflows_last_steps,
    # types / categories
    "get_workitem_types": tool_get_workitem_types,
    "get_entity_custom_fields": tool_get_entity_custom_fields,
    "get_story_categories": tool_get_story_categories,
    # test cases
    "get_tcases": tool_get_tcases,
    "create_tcases": tool_create_tcases,
    "create_tcases_batch": tool_create_tcases_batch,
    "get_tcases_count": tool_get_tcases_count,
    "get_tcases_custom_fields": tool_get_tcases_custom_fields,
    # wiki
    "get_wikis": tool_get_wikis,
    "create_wiki": tool_create_wiki,
    "get_wiki_count": tool_get_wiki_count,
    # user / todo
    "get_user_info": tool_get_user_info,
    "get_todo": tool_get_todo,
    "get_user_story_todo": tool_get_user_story_todo,
    "get_user_bug_todo": tool_get_user_bug_todo,
    "get_user_task_todo": tool_get_user_task_todo,
    # timesheets
    "update_timesheets": tool_update_timesheets,
    "get_timesheets": tool_get_timesheets,
    # scm / release / relations
    "get_scm_copy_keywords": tool_get_scm_copy_keywords,
    "get_release_info": tool_get_release_info,
    "add_entity_relations": tool_add_entity_relations,
    # qiwei
    "send_qiwei_message": tool_send_qiwei_message,
}


def _read_params() -> dict:
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                raw = f.read().strip()
            if not raw:
                return {}
            try:
                return json.loads(raw)
            except json.JSONDecodeError as e:
                print(_err(f"JSON parse error: {e}"))
                sys.exit(1)
    return {}


def main():
    if len(sys.argv) < 2 or sys.argv[1] in ("-h", "--help", "help"):
        print(json.dumps({
            "usage": "python tapd.py <action> <params.json>",
            "actions": sorted(TOOLS.keys()),
            "count": len(TOOLS),
        }, ensure_ascii=False, indent=2))
        return

    action = sys.argv[1]
    if action not in TOOLS:
        print(_err(f"unknown action: {action}, available: {', '.join(sorted(TOOLS.keys()))}"))
        sys.exit(1)

    args = _read_params()

    try:
        client = TAPDClient()
        result = TOOLS[action](client, args)
        print(result)
    except Exception as e:
        print(_err(f"执行失败 [{action}]: {e}"))
        sys.exit(1)


if __name__ == "__main__":
    main()
