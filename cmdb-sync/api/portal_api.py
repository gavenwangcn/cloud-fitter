import requests
import hashlib
import urllib.parse
from datetime import datetime, timezone


class PortalAPI:
    """Portal API客户端"""
    def __init__(self, base_url: str = "http://doserver:80"):
        self.base_url = base_url
        self.secret_key = "g1BOI42T%XeA5Qbdqo6ZWxfkyDHuph0K"
    
    def generate_signature(self, params):
        # 按字典序排序并拼接字符串
        query_string = "&".join(
            f"{key}={urllib.parse.quote(str(params[key]), safe='')}"
            for key in sorted(params.keys())
        )
        
        sign_string = query_string + self.secret_key
        return hashlib.md5(sign_string.encode('utf-8')).hexdigest()
    

    def push_data_to_portal(self, params):        
        params["timestamp"] = int(datetime.now(timezone.utc).timestamp())
        params["signature"] = self.generate_signature(params)

        url = f"{self.base_url}/api/v1/third-party/data"
        response = requests.post(url, json=params)
        response.raise_for_status()
        return response.json()
    
    def get_data_from_portal(self, params):
        params["timestamp"] = int(datetime.now(timezone.utc).timestamp())
        params["signature"] = self.generate_signature(params)

        url = f"{self.base_url}/api/v1/cmdb/get_ci_query"
        url = f"https://devops-ditc.mychery.com/api/v1/cmdb/get_ci_query"
        response = requests.get(url, params=params)
        response.raise_for_status()
        return response.json()