import { request } from 'umi';

export async function queryAllRds() {
  return request('/apis/rds/all', {
    method: 'POST',
    data: {},
  });
}

export async function queryRdsByAccount(provider: number, accountName: string) {
  return request('/apis/rds/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}
