import requests
import time
import schedule
import logging
import os
from datetime import datetime
from api.cmdb_api import CMBDAPI, YJSCMDBAPI
from api.portal_api import PortalAPI
from api.teambition_api import TeamBitionAPI

# 配置日志
log_file = os.path.join(os.path.dirname(__file__), 'cmdb.log')
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler(log_file, encoding='utf-8'),  
        logging.StreamHandler() 
    ]
)
logger = logging.getLogger(__name__)


def cmdb():
    try:
        yjsclient = YJSCMDBAPI()
        client = CMBDAPI()

        # 获取所有系统
        page = 1
        system_ids = []
        while True:
            payload = {
                "q": "_type:system",
                "page": page
            }
            result = client.get_ci(payload)
            if result["total"] == 0:
                break
            else:
                system_ids.extend([item["system_id"] for item in result["result"]])
                page += 1
        
        for system_id in system_ids:
            logger.info("==> %s", yjsclient.get_system_info(system_id))
    except Exception as e:
        logger.error("发生错误: %s", e)

    # try:
    #     client = CMBDAPI()
    #     payload = {
    #         "q": "_type:middle_software,sys_node_name:华为公有云-cn-east-4,system_id:10000483,resource_name:股份OTD订单中心-生产-经销商门户与DMS"
    #         }
    #     data = client.get_ci(payload)
    #     print("data==>", data)
    #     exit()
    #     for type in ["product", "sub_product", "projects", "project", "system"]:
    #         client.cmdb_2_tb(type)
    # except Exception as e:
    #     logger.error("发生错误: %s", e)



def teambition():
    try:
        client = TeamBitionAPI()        
        client.get_all_tasks()        
    except requests.exceptions.HTTPError as e:
        logger.error("HTTP错误: %s", e)
        if e.response.status_code == 401:
            logger.error("认证失败，请检查访问令牌是否正确")
    except Exception as e:
        logger.error("发生错误: %s", e)


def portal():
    try:
        client = PortalAPI()
        payload = {
            "value": "10000483",
            "ci_type": "system"
        }
        client.get_data_from_portal(payload)
    except Exception as e:
        logger.error("发生错误: %s", e)

if __name__ == "__main__":
    cmdb()
    exit()  
    schedule.every().day.at("23:30").do(cmdb)
    schedule.every().day.at("11:30").do(teambition)    
    logger.info("当前时间: %s，定时任务已启动...", datetime.now().strftime('%Y-%m-%d %H:%M:%S'))

    while True:
        schedule.run_pending()
        time.sleep(60)