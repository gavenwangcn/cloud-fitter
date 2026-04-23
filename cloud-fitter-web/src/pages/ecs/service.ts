import { request } from 'umi';

export async function queryAllEcs() {
  return request('/apis/ecs/all', {
    method: 'POST',
    data: {},
    /** 华为等多区域聚合 + 每实例查块设备，后端常需 20s+ */
    timeout: 120000,
  });
}
