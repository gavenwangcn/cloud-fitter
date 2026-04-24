import requests
import jwt
from datetime import datetime, timedelta, timezone
from typing import List, Dict, Optional
from api.portal_api import PortalAPI
import logging


class TeamBitionAPI:
    """TeamBition API客户端"""
    
    def __init__(self, base_url: str = "https://open.teambition.com"):
        APP_ID = "67d004a98caf65b67508a794"
        SECRET_KEY = "0N6Ytug0D96i2TP77eYpKKG9DG3b0dWD"
        EXPIRES_IN_HOURS = 24

        payload = {
            "exp": datetime.now(timezone.utc) + timedelta(hours=EXPIRES_IN_HOURS),  
            "iat": datetime.now(timezone.utc),                                       
            "_appId": APP_ID                                                         
        }
        
        self.access_token = jwt.encode(payload, SECRET_KEY, algorithm="HS256")    
        if isinstance(self.access_token, bytes):
            self.access_token = self.access_token.decode('utf-8')
        self.base_url = base_url
        self.headers = {
            "Authorization": f"Bearer {self.access_token}",
            "Content-Type": "application/json",
            "X-Tenant-Id": "67318fa18deba4b7a3b5de17",
            "X-Tenant-Type": "organization",
            "X-Operator-Id": "67d9376701949dd3661c61b3"
        }


    def cmdb_2_tb_product_line(self, cfID, choices):
        url = f"{self.base_url}/api/v3/customfield/{cfID}/update"
        payload = {
            "choices": choices
        }
        response = requests.put(url, headers=self.headers, json=payload)
        response.raise_for_status()

    
    def get_projects(self) -> List[Dict]:
        url = f"{self.base_url}/api/v3/project/query"
        params = {
            "pageSize": 10000
        }
        response = requests.get(url, headers=self.headers, params=params)
        response.raise_for_status()
        return response.json().get("result", [])
    

    def get_tasks_by_project(self, project_id: str, 
                           pageSize: int = 100, 
                           pageToken: str = "") -> List[Dict]:
        last_update_time = (datetime.now(timezone.utc) - timedelta(hours=25)).strftime('%Y-%m-%dT%H:%M:%S') + 'Z'
        url = f"{self.base_url}/api/v3/project/{project_id}/task/query"
        params = {
            "pageSize": pageSize,
            "pageToken": pageToken,
            "q": f"updated > '{last_update_time}'"
        }        
        response = requests.get(url, headers=self.headers, params=params)
        response.raise_for_status()
        return response.json()
        

    def get_all_tasks(self, project_id: Optional[int] = None) -> List[Dict]:   
        if project_id:
            # 获取单个项目的所有任务
            pageSize = 100
            pageToken = ""
            while True:
                tasks = self.get_tasks_by_project(project_id, pageSize, pageToken)
                if len(tasks.get("result")) == 0:
                    break
                for task in tasks.get("result"):
                    if task["customfields"]: 
                        self.process_task(task["customfields"])
                    else:
                        continue
                pageToken = tasks.get("nextPageToken", "")
                if not pageToken:
                    break
        else:
            # 获取所有项目的所有任务
            projects = self.get_projects()
            for project in projects:
                project_id = project["id"]
                logging.info("正在获取项目: %s", project["name"])
                
                pageSize = 100
                pageToken = ""
                while True:
                    tasks = self.get_tasks_by_project(project_id, pageSize, pageToken)
                    if len(tasks.get("result")) == 0:
                        break
                    for task in tasks.get("result"):
                        if task["customfields"]: 
                            self.process_task(task["customfields"])
                        else:
                            continue
                    pageToken = tasks.get("nextPageToken", "")
                    if not pageToken:
                        break
    

    def process_task(self, data):
        cfid_mapping = {
            "697ff55b993eb79daf9f33a2": "product",
            "69819f00f565ef30d43a7992": "sub_product",
            "69819f785aa804e357918f42": "project",
            "69819f8886d948e828b33218": "project_set",
            "69819f92c57bebf644aff7ce": "system"
        }

        # 创建查找字典
        title_lookup = {}
        for item in data:
            if item.get("value") and len(item["value"]) > 0:
                title_lookup[item["cfId"]] = item["value"][0].get("title", "")
        result = {
            name: title_lookup.get(cfid, "")
            for cfid, name in cfid_mapping.items()
        }

        # 保存到Portal
        portal_client = PortalAPI()
        portal_client.push_data_to_portal(result)