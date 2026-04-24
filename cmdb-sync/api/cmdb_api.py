import os
from typing import Optional

import requests
import hashlib
import urllib.parse
import uuid
import logging
from pathlib import Path

from dotenv import load_dotenv

from api.teambition_api import TeamBitionAPI

# 仓库根目录 .env（与 cloud-fitter Go 的 CLOUD_FITTER_CMDB_* 一致）
_repo_root = Path(__file__).resolve().parents[2]
load_dotenv(_repo_root / ".env")

logger = logging.getLogger(__name__)

class CMBDAPI:
    """CMDB API客户端"""
    def __init__(self, base_url: Optional[str] = None):
        self.base_url = (base_url or os.environ.get("CLOUD_FITTER_CMDB_BASE_URL", "")).rstrip("/")
        self.key = os.environ.get("CLOUD_FITTER_CMDB_KEY", "")
        self.secret = os.environ.get("CLOUD_FITTER_CMDB_SECRET", "")
        if not self.base_url or not self.key or not self.secret:
            raise ValueError(
                "CMDB 未配置：请在仓库根目录 .env 中设置 "
                "CLOUD_FITTER_CMDB_BASE_URL、CLOUD_FITTER_CMDB_KEY、CLOUD_FITTER_CMDB_SECRET"
            )
    
    def build_api_key(self, path, params):
        """生成CMDB API签名"""
        values = "".join([str(params[k]) for k in sorted((params or {}).keys())
                      if k not in ("_key", "_secret") and not isinstance(params[k], (dict, list))])
        _secret = "".join([path, self.secret, values]).encode("utf-8")
        params["_secret"] = hashlib.sha1(_secret).hexdigest()
        params["_key"] = self.key        
        return params
    
    def get_ci(self, payload):
        URL = f"{self.base_url}/api/v0.1/ci/s"
        payload = self.build_api_key(urllib.parse.urlparse(URL).path, payload)
        return requests.get(URL, params=payload).json()
    
    def get_ci_id(self, payload):
        data = self.get_ci(payload)
        if data and data.get("result"):
            return data.get("result")[0].get("_id")
        return None
    
    def get_system_level_relations(self, payload):
        URL = f"{self.base_url}/api/v0.1/ci_relations/s"
        payload = self.build_api_key(urllib.parse.urlparse(URL).path, payload)
        return requests.get(URL, params=payload).json()
    
    def add_ci(self, payload):
        URL = f"{self.base_url}/api/v0.1/ci"
        payload = self.build_api_key(urllib.parse.urlparse(URL).path, payload)
        return requests.post(URL, json=payload).json()
    
    
    def cmdb_2_tb(self, type): 
        tb_client = TeamBitionAPI()
        if type == "product":
            pass
        elif type == "sub_product":
            pass
        elif type == "projects":
            pass
        elif type == "project":
            pass
        elif type == "system":
            pass
        else:
            raise ValueError(f"Unsupported type: {type}")
        

class YJSCMDBAPI:
    """云计算的CMDB API客户端"""
    def __init__(self, base_url: str = "http://10.2.6.124:5000"):
        self.base_url = base_url

    def get_system_all_resources(self, system_id):
        url = f"{self.base_url}/api/system/resources"
        data = {
            "sys_id": system_id
        }
        response = requests.post(url, json=data)
        if response.status_code == 404:
            return "SYSTEM NOT FOUND!!"
        return response.json()

    def get_system_info(self, system_id):
        target_labels = {'CCE_CLUSTER', 'ECS', 'RDS_INS', 'DCS_REDIS', 'DMS_ROCKETMQ', 'SYSTEM'}
        system_name = set()
        system_nodes = set()
        k8s_clusters = set()
        hosts = set()
        middlewares = set()
        data = self.get_system_all_resources(system_id)
        if data == "SYSTEM NOT FOUND!!":
            logger.info(f"SYSTEM {system_id} NOT FOUND!!")
            return

        for resource in data.get('related_resources', []):
            for node in resource.get('nodes', []):
                node_labels = set(node.get('labels', []))
                if not node_labels & target_labels:  # 无交集则跳过
                    continue
                props = node.get('properties', {})

                # 获取系统节点名称
                location = props.get('location')
                region = props.get('region_id')
                if location and region:  # 两者均非空
                    system_nodes.add(location+"-"+region)

                # 获取系统名称
                system = props.get('sys_fullname')
                if system:
                    system_name.add(system.split('-')[1])

                # 获取K8S集群
                if node.get('labels', []) == ['CCE_CLUSTER']:
                    k8s_temp = node.get('properties', {})
                    k8s_cluster_name = k8s_temp.get('name', '')
                    k8s_clusters_version = k8s_temp.get('clusterVersion', '')
                    k8s_clusters_location = k8s_temp.get('location', '')
                    k8s_clusters_region = k8s_temp.get('region_id', '')
                    k8s_cluster_id = k8s_temp.get('id', '')                    
                    k8s_clusters.add((k8s_cluster_name, k8s_clusters_version, k8s_clusters_location, k8s_clusters_region, k8s_cluster_id))

                # 获取主机
                if node.get('labels', []) == ['ECS']:
                    host_temp = node.get('properties', {})
                    host_name = host_temp.get('host_name', '')
                    host_ip = host_temp.get('ip_addresses', [])[0].split(':')[-1]
                    host_location = host_temp.get('location', '')
                    host_region = host_temp.get('region_id', '')
                    host_cpu = host_temp.get('vcpus', '')
                    host_mem = host_temp.get('mem_G', '')
                    host_os = host_temp.get('os_images', '')
                    host_k8s_id = host_temp.get('cce_cluster_id', '')
                    hosts.add((host_name, host_ip, host_location, host_region, host_cpu, host_mem, host_os, host_k8s_id))

                # 获取中间件
                if set(node.get('labels', [])).issubset(set(['RDS_INS', 'DCS_REDIS', 'DMS_ROCKETMQ'])):
                    middleware_temp = node.get('properties', {})
                    middleware_name = middleware_temp.get('name', '')
                    middleware_type = node.get('labels', [])[0]
                    middleware_ip = middleware_temp.get('ip', '')
                    middleware_cpu = middleware_temp.get('cpu', '')
                    middleware_mem = middleware_temp.get('mem_G', '')
                    middleware_location = middleware_temp.get('location', '')
                    middleware_region = middleware_temp.get('region_id', '')
                    middlewares.add((middleware_name, middleware_type, middleware_ip, middleware_cpu, middleware_mem, middleware_location, middleware_region))

        
        self.add_cmdb_system_nodes(system_id, system_name, system_nodes)

        cluster_to_ecs = self.host_belongs_k8s(k8s_clusters, hosts)
        self.add_cmdb_k8s_clusters(system_id, k8s_clusters, cluster_to_ecs)

        self.add_cmdb_hosts(system_id, hosts)

        self.add_cmdb_middlewares(system_id, middlewares)

    def add_cmdb_system_nodes(self, system_id, system_name, system_nodes):
        client = CMBDAPI()
        for node in system_nodes:
            # 查询系统节点是否已存在
            payload = {
                "q": f"_type:system_node,sys_node_name:{node},system_id:{system_id}"
            }
            id = client.get_ci_id(payload)
            if id:
                logger.info(f"系统节点 {node} 已存在，ID: {id}")
                continue
            else:                
                payload = {
                    "q": f"_type:system,system_id:{system_id}"
                }
                id = client.get_ci_id(payload)
                payload = {
                    "root_id": f"{id}",
                    "level": "1,2,3",
                    "reverse": 1
                }
                data = client.get_system_level_relations(payload)

                if data.get("result"):
                    for item in data.get("result"):
                        if item.get("ci_type") == "biz_domain":
                            biz_domain = item.get("biz_domain_name")
                        elif item.get("ci_type") == "product_line":
                            product = item.get("product_line_name")
                        elif item.get("ci_type") == "product":
                            sub_product = item.get("product_name")
                else:
                    logger.error(f"系统 {system_id} 系统节点 {node} 未找到相关关系")
                    continue

                try:
                    parts = node.split('-', 1)
                    payload = {
                        "uuid": str(uuid.uuid4()),
                        "ci_type": "system_node",
                        "sys_node_name": node,
                        "system_id": system_id,
                        "system_name": list(system_name)[0],
                        "product_name": sub_product,
                        "product_line_name": product,
                        "biz_domain_name": biz_domain,
                        "cloud_type": parts[0],
                        "location": parts[1]                    
                    }                
                    data = client.add_ci(payload)
                    logger.info(f"添加系统节点成功: {node}, 响应: {data}")
                except Exception as e:
                    logger.error(f"添加系统节点失败: {e}")

    def add_cmdb_k8s_clusters(self, system_id,  k8s_clusters, cluster_to_ecs):
        client = CMBDAPI()
        for cluster in k8s_clusters:
            # 查询K8S集群是否已存在
            payload = {
                "q": f"_type:k8s_cluster,k8s_cluster_name:{cluster[0]},system_id:{system_id},sys_node_name:{cluster[2]}-{cluster[3]}"
            }
            id = client.get_ci_id(payload)
            if id:
                logger.info(f"K8S集群 {cluster[0]} 已存在，ID: {id}")
                continue
            else:              
                payload = {
                    "uuid": str(uuid.uuid4()),
                    "k8s_uuid": str(uuid.uuid4()),
                    "ci_type": "k8s_cluster",
                    "system_id": system_id,
                    "sys_node_name": cluster[2]+"-"+cluster[3],
                    "k8s_cluster_name": cluster[0],
                    "host_ip_new": ','.join(cluster_to_ecs.get(cluster[0], [])),
                    "k8s_version": cluster[1],                    
                    "cloud_type": cluster[2],
                    "location": cluster[3]
                }
                try:
                    data = client.add_ci(payload)
                    logger.info(f"添加K8S集群成功: {cluster[0]}, 响应: {data}")
                except Exception as e:
                    logger.error(f"添加K8S集群失败: {e}")

    def add_cmdb_hosts(self, system_id, hosts):
        client = CMBDAPI()
        for host in hosts:
            # 查询主机是否已存在
            payload = {
                "q": f"_type:server,sys_node_name:{host[2]}-{host[3]},system_id:{system_id},private_ip:{host[1]}"
            }
            id = client.get_ci_id(payload)
            if id:
                logger.info(f"主机 {host[0]}: {host[1]} 已存在，ID: {id}")
                continue
            else:                
                payload = {
                    "uuid": str(uuid.uuid4()),
                    "ci_type": "server",
                    "system_id": system_id,
                    "sys_node_name": host[2]+"-"+host[3],
                    "server_name": host[0],
                    "private_ip": host[1],
                    "cpu_count": int(host[4]),
                    "ram_size": host[5],
                    "os_version": host[6],
                    "location": host[3],
                    "cloud_type": host[2]
                }
                try:
                    data = client.add_ci(payload)
                    logger.info(f"添加主机成功: {host[0]}, 响应: {data}")
                except Exception as e:
                    logger.error(f"添加主机失败: {e}")

    def add_cmdb_middlewares(self, system_id, middlewares):
        client = CMBDAPI()
        for middleware in middlewares:
            # 查询中间件是否已存在
            payload = {
                "q": f"_type:middle_software,sys_node_name:{middleware[5]}-{middleware[6]},system_id:{system_id},resource_name:{middleware[0]}"
            }
            id = client.get_ci_id(payload)
            if id:
                logger.info(f"中间件 {middleware[0]} 已存在，ID: {id}")
                continue
            else:                
                payload = {
                    "uuid": str(uuid.uuid4()),
                    "ci_type": "middle_software",
                    "system_id": system_id,
                    "sys_node_name": middleware[5]+"-"+middleware[6],
                    "resource_name": middleware[0],
                    "resource_type": middleware[1],
                    "location": middleware[5],
                    "cloud_type": middleware[6],
                    "private_ip": middleware[2],
                    "cpu_count": middleware[3],
                    "ram_size": middleware[4]
                }
                try:
                    data = client.add_ci(payload)
                    logger.info(f"添加中间件成功: {middleware[0]}, 响应: {data}")
                except Exception as e:
                    logger.error(f"添加中间件失败: {e}")

    def host_belongs_k8s(self, k8s_clusters, hosts):
        # K8S集群与主机关联
        cluster_dict = {c[4]: c[0] for c in k8s_clusters} 

        cluster_to_ecs = {}
        for ecs in hosts:
            cce_id = ecs[7]  
            if cce_id and cce_id in cluster_dict:  # 忽略空串和不存在的ID
                cluster_name = cluster_dict[cce_id]
                if cluster_name not in cluster_to_ecs:
                    cluster_to_ecs[cluster_name] = []
                cluster_to_ecs[cluster_name].append(ecs[1])

        return cluster_to_ecs
