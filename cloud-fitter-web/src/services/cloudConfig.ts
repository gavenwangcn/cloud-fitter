import { request } from 'umi';

export interface CloudConfigRow {
  id: number;
  provider: number;
  name: string;
}

export async function listCloudConfigs(params?: {
  page?: number;
  pageSize?: number;
}): Promise<{ configs: CloudConfigRow[]; total?: number }> {
  return request('/apis/configs', { method: 'GET', params: params || {} });
}

export async function deleteCloudConfig(id: number) {
  return request(`/apis/configs?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function createCloudConfig(data: {
  provider: number;
  name: string;
  accessId: string;
  accessSecret: string;
}) {
  return request('/apis/configs', { method: 'POST', data });
}

export function providerLabel(p: number): string {
  switch (p) {
    case 0:
      return '阿里云';
    case 1:
      return '腾讯云';
    case 2:
      return '华为云';
    default:
      return `云(${p})`;
  }
}
