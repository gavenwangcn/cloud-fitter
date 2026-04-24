import { request } from 'umi';

export async function queryCceByAccount(provider: number, accountName: string) {
  return request('/apis/cce/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}

export async function queryCceBySystem(systemName: string) {
  return request('/apis/cce/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: 120000,
  });
}
