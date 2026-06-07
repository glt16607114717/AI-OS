import os
import json
import requests
from base64 import b64encode
from pathlib import Path
from typing import Optional, Dict, Any, List


class TAPDClient:

    def __init__(self, config_path: Optional[str] = None):
        if config_path:
            config_file = Path(config_path)
        else:
            config_file = Path(__file__).parent / "tapd_config.json"
        self._load_config(config_file)
        self._setup_auth()

    def _load_config(self, config_file: Path) -> None:
        if config_file.exists():
            with open(config_file, "r", encoding="utf-8") as f:
                cfg = json.load(f)
        else:
            cfg = {}
        self.api_base_url = (
            cfg.get("api_base_url")
            or os.getenv("TAPD_API_BASE_URL", "https://api.tapd.cn")
        )
        self.tapd_base_url = (
            cfg.get("tapd_base_url")
            or os.getenv("TAPD_BASE_URL", "https://www.tapd.cn")
        )
        self.bot_url = cfg.get("bot_url") or os.getenv("BOT_URL", "")
        self.access_token = cfg.get("access_token") or os.getenv("TAPD_ACCESS_TOKEN", "")
        self.api_user = cfg.get("api_user") or os.getenv("TAPD_API_USER", "")
        self.api_password = (
            cfg.get("api_password") or os.getenv("TAPD_API_PASSWORD", "")
        )
        self.nick: Optional[str] = None

    def _setup_auth(self) -> None:
        if self.access_token:
            self.headers = {
                "Authorization": f"Bearer {self.access_token}",
                "Content-Type": "application/json",
                "Via": "script",
            }
            self.nick = self.get_user_info()
        elif self.api_user and self.api_password:
            auth_str = f"{self.api_user}:{self.api_password}"
            self.headers = {
                "Authorization": f"Basic {b64encode(auth_str.encode()).decode()}",
                "Content-Type": "application/json",
                "Via": "script",
            }
        else:
            raise ValueError(
                "未配置 TAPD 认证信息，请在 tapd_config.json 或环境变量中配置"
            )

    def _make_request(
        self,
        method: str,
        endpoint: str,
        params: Optional[Dict] = None,
        data: Optional[Dict] = None,
    ) -> Dict:
        url = f"{self.api_base_url}/{endpoint}"
        separator = "&" if "?" in url else "?"
        url = f"{url}{separator}s=script"
        if params and method.upper() == "GET":
            time_fields = {"created", "modified", "begin", "due", "completed",
                           "startdate", "enddate", "deadline", "plan_start_time",
                           "plan_end_time", "start_time", "end_time"}
            safe_params = {}
            for k, v in params.items():
                if k in time_fields:
                    url += f"&{k}={v}"
                else:
                    safe_params[k] = v
            params = safe_params if safe_params else None
        response = requests.request(
            method=method,
            url=url,
            headers=self.headers,
            params=params,
            json=data,
            timeout=30,
        )
        response.raise_for_status()
        return response.json()

    def is_cloud_env(self) -> bool:
        return "api.tapd.cn" in self.api_base_url

    def _to_long_id(self, short_id: str, workspace_id: str) -> str:
        pre_id = "11" if self.is_cloud_env() else "10"
        if short_id.isdigit() and len(short_id) <= 9:
            return f"{pre_id}{workspace_id}{short_id.zfill(9)}"
        return short_id

    def _convert_ids(self, id_val: str, workspace_id: str) -> str:
        if "," in id_val:
            return ",".join(
                self._to_long_id(i.strip(), workspace_id)
                for i in id_val.split(",")
            )
        return self._to_long_id(id_val, workspace_id)

    def get_user_info(self) -> Optional[str]:
        resp = self._make_request("GET", "users/info")
        data = resp.get("data")
        if isinstance(data, dict):
            return data.get("nick")
        return None

    def get_user_participant_projects(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request(
            "GET", "workspaces/user_participant_projects", params=params
        )

    def get_workspace_info(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = params.get("workspace_id") if params else None
        return self._make_request(
            "GET",
            f"workspaces/get_workspace_info?workspace_id={workspace_id}",
        )

    def check_mini_project(self, workspace_id: int) -> bool:
        ret = self.get_workspace_info({"workspace_id": workspace_id})
        return (
            ret.get("data", {}).get("Workspace", {}).get("category") == "mini_project"
        )

    def get_stories(self, params: Optional[Dict] = None) -> Dict:
        entity_type = "stories"
        if params and params.get("entity_type") == "tasks":
            entity_type = "tasks"
        default_params = {"page": 1, "limit": 10}
        if params:
            default_params.update(params)
        if "id" in default_params and "workspace_id" in default_params:
            default_params["id"] = self._convert_ids(
                str(default_params["id"]), str(default_params["workspace_id"])
            )
        default_params.pop("entity_type", None)
        return self._make_request("GET", entity_type, params=default_params)

    def create_or_update_story(self, params: Dict[str, Any]) -> Dict:
        entity_type = "stories"
        if params.get("entity_type") == "tasks":
            entity_type = "tasks"
        params.pop("entity_type", None)
        if "id" in params and "workspace_id" in params:
            id_val = str(params["id"])
            if id_val.isdigit() and len(id_val) <= 9:
                params["id"] = self._to_long_id(
                    id_val, str(params["workspace_id"])
                )
        if self.nick:
            if "id" in params:
                params["current_user"] = self.nick
            else:
                params["creator"] = self.nick
        return self._make_request("POST", entity_type, data=params)

    def get_story_count(self, params: Optional[Dict] = None) -> Dict:
        entity_type = "stories"
        if params and params.get("entity_type") == "tasks":
            entity_type = "tasks"
        clean = {k: v for k, v in (params or {}).items() if k != "entity_type"}
        return self._make_request("GET", f"{entity_type}/count", params=clean)

    def get_stories_custom_fields_settings(
        self, params: Optional[Dict] = None
    ) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        return self._make_request(
            "GET",
            f"stories/custom_fields_settings?workspace_id={workspace_id}",
        )

    def get_stories_fields_label(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        return self._make_request(
            "GET", f"stories/get_fields_lable?workspace_id={workspace_id}"
        )

    def get_stories_fields_info(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        return self._make_request(
            "GET", f"stories/get_fields_info?workspace_id={workspace_id}"
        )

    def get_related_bugs(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "stories/get_related_bugs", params=params)

    def get_bugs(self, params: Optional[Dict] = None) -> Dict:
        default_params = {"page": 1, "limit": 10}
        if params:
            default_params.update(params)
        if "id" in default_params and "workspace_id" in default_params:
            default_params["id"] = self._convert_ids(
                str(default_params["id"]), str(default_params["workspace_id"])
            )
        return self._make_request("GET", "bugs", params=default_params)

    def create_or_update_bug(self, params: Dict[str, Any]) -> Dict:
        if "id" in params and "workspace_id" in params:
            id_val = str(params["id"])
            if id_val.isdigit() and len(id_val) <= 9:
                params["id"] = self._to_long_id(
                    id_val, str(params["workspace_id"])
                )
        if self.nick:
            if "id" in params:
                params["current_user"] = self.nick
            else:
                params["reporter"] = self.nick
        return self._make_request("POST", "bugs", data=params)

    def get_bug_count(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "bugs/count", params=params)

    def get_bug_custom_fields(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        return self._make_request(
            "GET", f"bugs/custom_fields_settings?workspace_id={workspace_id}"
        )

    def get_comments(self, params: Optional[Dict] = None) -> Dict:
        default_params = {"page": 1, "limit": 30}
        if params:
            default_params.update(params)
        return self._make_request("GET", "comments", params=default_params)

    def create_comments(self, params: Dict[str, Any]) -> Dict:
        if "entry_id" in params and "workspace_id" in params:
            id_val = str(params["entry_id"])
            params["entry_id"] = self._to_long_id(
                id_val, str(params["workspace_id"])
            )
        if "entry_type" not in params:
            params["entry_type"] = "bug"
        if self.nick:
            if "id" in params:
                params["change_creator"] = self.nick
            else:
                params.setdefault("author", self.nick)
        return self._make_request("POST", "comments", data=params)

    def get_image(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "files/get_image", params=params)

    def get_attachments(self, params: Optional[Dict] = None) -> Dict:
        default_params = {}
        if params:
            default_params.update(params)
        result = self._make_request("GET", "attachments", params=default_params)
        if result.get("status") == 1 and result.get("data"):
            workspace_id = default_params.get("workspace_id")
            if workspace_id:
                for item in result["data"]:
                    if "Attachment" in item:
                        attachment_id = item["Attachment"].get("id")
                        if attachment_id:
                            try:
                                dl = self.get_attachment_download_url(
                                    {
                                        "id": attachment_id,
                                        "workspace_id": workspace_id,
                                    }
                                )
                                if dl.get("status") == 1 and dl.get("data"):
                                    data = dl["data"]
                                    url = None
                                    if isinstance(data, dict):
                                        if "Attachment" in data and isinstance(
                                            data["Attachment"], dict
                                        ):
                                            url = data["Attachment"].get(
                                                "download_url"
                                            )
                                        else:
                                            url = data.get("download_url")
                                    item["Attachment"]["download_url"] = url
                                else:
                                    item["Attachment"]["download_url"] = None
                            except Exception:
                                item["Attachment"]["download_url"] = None
        return result

    def get_attachment_download_url(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "attachments/down", params=params)

    def get_iterations(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        query = f"?workspace_id={workspace_id}"
        if params and "id" in params:
            query += f"&id={params['id']}"
        if params and "name" in params:
            query += f"&name={params['name']}"
        return self._make_request("GET", f"iterations{query}")

    def create_or_update_iteration(self, params: Dict[str, Any]) -> Dict:
        if self.nick:
            if "id" in params:
                params["current_user"] = self.nick
            else:
                params["creator"] = self.nick
        return self._make_request("POST", "iterations", data=params)

    def get_workflows_all_transitions(self, params: Dict[str, Any]) -> Dict:
        workspace_id = params.get("workspace_id", "")
        system = params.get("system", "")
        query = f"?workspace_id={workspace_id}&system={system}"
        if "workitem_type_id" in params:
            query += f"&workitem_type_id={params['workitem_type_id']}"
        return self._make_request("GET", f"workflows/all_transitions{query}")

    def get_workflows_status_map(self, params: Dict[str, Any]) -> Dict:
        workspace_id = params.get("workspace_id", "")
        system = params.get("system", "")
        query = f"?workspace_id={workspace_id}&system={system}"
        if "workitem_type_id" in params:
            query += f"&workitem_type_id={params['workitem_type_id']}"
        return self._make_request("GET", f"workflows/status_map{query}")

    def get_workflows_last_steps(self, params: Dict[str, Any]) -> Dict:
        workspace_id = params.get("workspace_id", "")
        system = params.get("system", "")
        query = f"?workspace_id={workspace_id}&system={system}"
        if "workitem_type_id" in params:
            query += f"&workitem_type_id={params['workitem_type_id']}"
        if "type" in params:
            query += f"&type={params['type']}"
        return self._make_request("GET", f"workflows/last_steps{query}")

    def get_workitem_types(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        return self._make_request(
            "GET", f"workitem_types?workspace_id={workspace_id}"
        )

    def get_entity_custom_fields(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        entity_type = (params or {}).get("entity_type", "stories")
        return self._make_request(
            "GET",
            f"{entity_type}/custom_fields_settings?workspace_id={workspace_id}",
        )

    def get_story_categories(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "story_categories", params=params)

    def get_tcases(self, params: Optional[Dict] = None) -> Dict:
        default_params = {"page": 1, "limit": 30}
        if params:
            default_params.update(params)
        return self._make_request("GET", "tcases", params=default_params)

    def create_tcases(self, params: Dict[str, Any]) -> Dict:
        if self.nick:
            if "id" in params:
                params["modifier"] = self.nick
            else:
                params["creator"] = self.nick
        return self._make_request("POST", "tcases", data=params)

    def create_tcases_batch(self, params: Dict[str, Any]) -> Dict:
        cases = params.get("cases", [])
        if self.nick:
            for case in cases:
                case.setdefault("creator", self.nick)
        return self._make_request("POST", "tcases/batch_save", data=cases)

    def get_tcases_count(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "tcases/count", params=params)

    def get_tcases_custom_fields(self, params: Optional[Dict] = None) -> Dict:
        workspace_id = (params or {}).get("workspace_id", "")
        return self._make_request(
            "GET", f"tcases/custom_fields_settings?workspace_id={workspace_id}"
        )

    def get_wikis(self, params: Optional[Dict] = None) -> Dict:
        default_params = {"page": 1, "limit": 30}
        if params:
            default_params.update(params)
        return self._make_request("GET", "tapd_wikis", params=default_params)

    def create_wiki(self, params: Dict[str, Any]) -> Dict:
        if self.nick:
            if "id" in params:
                params["modifier"] = self.nick
            else:
                params["creator"] = self.nick
        return self._make_request("POST", "tapd_wikis", data=params)

    def get_wiki_count(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "tapd_wikis/count", params=params)

    def get_todo(self, params: Dict[str, Any]) -> Dict:
        entity_type = params.get("entity_type", "bug")
        user_nick = self.nick or params.get("user_nick", "")
        return self._make_request(
            "GET", f"users/todo/{user_nick}/{entity_type}"
        )

    def get_user_story_todo(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request(
            "GET", "user_oauth/get_user_todo_story", params=params
        )

    def get_user_bug_todo(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request(
            "GET", "user_oauth/get_user_todo_bug", params=params
        )

    def get_user_task_todo(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request(
            "GET", "user_oauth/get_user_todo_task", params=params
        )

    def update_timesheets(self, params: Dict[str, Any]) -> Dict:
        if self.nick:
            params["owner"] = self.nick
        return self._make_request("POST", "timesheets", data=params)

    def get_timesheets(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "timesheets", params=params)

    def get_scm_copy_keywords(self, params: Dict[str, Any]) -> Dict:
        if "object_id" in params and "workspace_id" in params:
            params["object_id"] = self._to_long_id(
                str(params["object_id"]), str(params["workspace_id"])
            )
        return self._make_request(
            "GET", "svn_commits/get_scm_copy_keywords", params=params
        )

    def get_release_info(self, params: Optional[Dict] = None) -> Dict:
        return self._make_request("GET", "releases", params=params)

    def add_entity_relations(self, params: Dict[str, Any]) -> Dict:
        return self._make_request("POST", "relations", data=params)

    def send_message(self, params: Dict[str, Any]) -> str:
        msg = params.get("msg", "")
        if "@" in msg:
            chat_data = {
                "msgtype": "markdown",
                "markdown": {"content": msg},
            }
        else:
            chat_data = {
                "msgtype": "markdown_v2",
                "markdown_v2": {"content": msg},
            }
        headers = {"Content-Type": "application/json"}
        response = requests.post(
            url=self.bot_url,
            headers=headers,
            json=chat_data,
            timeout=500,
        )
        return response.text

    def get_story_or_task_url_template(
        self, workspace_id: int, entity_type: str
    ) -> str:
        is_mini = self.check_mini_project(workspace_id)
        if entity_type == "tasks":
            return f"{self.tapd_base_url}/{workspace_id}/prong/tasks/view/{{id}}"
        if is_mini:
            return f"{self.tapd_base_url}/tapd_fe/t/index/{workspace_id}?workitemId={{id}}"
        return f"{self.tapd_base_url}/{workspace_id}/prong/stories/view/{{id}}"

    def filter_fields(
        self, data_list: list, fields_param=None
    ) -> list:
        if not data_list:
            return data_list
        if isinstance(fields_param, str):
            fields = [f.strip() for f in fields_param.split(",") if f.strip()]
        elif isinstance(fields_param, list):
            fields = fields_param
        else:
            fields = []
        filtered = []
        for item in data_list:
            if not isinstance(item, dict):
                filtered.append(item)
                continue
            type_key = None
            for key in ("Story", "Bug", "Task", "Iteration"):
                if key in item and isinstance(item[key], dict):
                    type_key = key
                    break
            obj = item[type_key] if type_key else item
            new_obj = {}
            for k, v in obj.items():
                if (
                    k.startswith("custom_field_")
                    and (v is None or v == "")
                    and (not fields_param or k not in fields)
                ):
                    continue
                if k.startswith("description") and type_key != "Iteration" and (
                    not fields_param or k not in fields
                ):
                    continue
                if k.startswith("custom_plan_field_") and v == "0":
                    continue
                new_obj[k] = v
            if type_key:
                filtered.append({type_key: new_obj})
            else:
                filtered.append(new_obj)
        return filtered

    def filter_fields_for_create_or_update(self, item: dict) -> dict:
        if not item or not isinstance(item, dict):
            return item
        type_key = None
        for key in ("Story", "Bug", "Task", "Iteration"):
            if key in item and isinstance(item[key], dict):
                type_key = key
                break
        obj = item[type_key] if type_key else item
        new_obj = {}
        for k, v in obj.items():
            if k.startswith("custom_field_") and (v is None or v == ""):
                continue
            if k.startswith("description") and type_key != "Iteration":
                continue
            if k.startswith("custom_plan_field_") and v == "0":
                continue
            new_obj[k] = v
        if type_key:
            return {type_key: new_obj}
        return new_obj
