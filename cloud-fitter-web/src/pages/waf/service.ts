import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

export async function queryWafByAccount(provider: number, accountName: string) {
  return request('/apis/waf/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}
