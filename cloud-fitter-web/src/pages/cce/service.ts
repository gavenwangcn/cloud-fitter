import { request } from 'umi';

export async function queryCceByAccount(provider: number, accountName: string) {
  return request('/apis/cce/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}
