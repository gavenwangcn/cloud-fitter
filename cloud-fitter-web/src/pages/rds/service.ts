import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
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
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

export async function queryRdsBySystem(systemName: string) {
  return request('/apis/rds/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}
