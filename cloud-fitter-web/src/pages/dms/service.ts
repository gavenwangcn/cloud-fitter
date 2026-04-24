import { request } from 'umi';

/** DMS（Kafka）按配置账号查询 */
export async function queryDmsByAccount(provider: number, accountName: string) {
  return request('/apis/kafka/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}

export async function queryDmsBySystem(systemName: string) {
  return request('/apis/kafka/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: 120000,
  });
}
