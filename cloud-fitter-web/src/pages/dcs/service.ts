import { request } from 'umi';

/** DCS（华为等）走后端 Redis 聚合接口 */
export async function queryAllDcs() {
  return request('/apis/redis/all', {
    method: 'POST',
    data: {},
  });
}

export async function queryDcsByAccount(provider: number, accountName: string) {
  return request('/apis/redis/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}

export async function queryDcsBySystem(systemName: string) {
  return request('/apis/redis/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: 120000,
  });
}
