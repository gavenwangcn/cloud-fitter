import { request } from 'umi';

export async function queryElbByAccount(provider: number, accountName: string) {
  return request('/apis/elb/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}

export async function queryElbBySystem(systemName: string) {
  return request('/apis/elb/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: 120000,
  });
}
