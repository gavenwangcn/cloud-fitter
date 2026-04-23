import { request } from 'umi';

export async function queryAllEcs() {
  return request('/apis/ecs/all', {
    method: 'POST',
    data: {},
    /** 华为等多区域聚合 + 每实例查块设备，后端常需 20s+ */
    timeout: 120000,
  });
}

/** 按配置中的账号名称拉取该账号下 ECS（与 SQLite / 内存租户一致） */
export async function queryEcsByAccount(provider: number, accountName: string) {
  return request('/apis/ecs/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}
