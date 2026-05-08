import { request } from 'umi';

export async function queryCertificatesByAccount(provider: number, accountName: string) {
  return request('/apis/certificates/by-account', {
    method: 'POST',
    data: { provider, accountName },
    timeout: 120000,
  });
}

export async function queryCertificatesBySystem(systemName: string) {
  return request('/apis/certificates/by-account', {
    method: 'POST',
    data: { systemName },
    timeout: 120000,
  });
}
