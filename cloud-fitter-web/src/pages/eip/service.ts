import { request } from 'umi';

export async function queryEipByAccount(provider: number, accountName: string) {
  return request('/apis/eip/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}

export async function queryEipBySystem(systemName: string) {
  return request('/apis/eip/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: 120000,
  });
}
