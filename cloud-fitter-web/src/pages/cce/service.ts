import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

export async function queryCceByAccount(provider: number, accountName: string) {
  return request('/apis/cce/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

export async function queryCceBySystem(systemName: string) {
  return request('/apis/cce/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}
