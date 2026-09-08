import { request } from 'umi';

export interface CloudConfigRow {
  id: number;
  provider: number;
  name: string;
  /** 0=国内 1=俄罗斯 2=土耳其，非华为云为 0 */
  huaweiAccountScope: number;
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
  huaweiAccountScope?: number;
}) {
  return request('/apis/configs', { method: 'POST', data });
}

export function providerLabel(p: number | string): string {
  switch (p) {
    case 0:
    case 'ali':
      return '阿里云';
    case 1:
    case 'tencent':
      return '腾讯云';
    case 2:
    case 'huawei':
      return '华为云';
    case 3:
    case 'aws':
      return 'AWS';
    default:
      return `云(${p})`;
  }
}

/** protojson 可能返回数字或字符串枚举名 */
export function isHuaweiProvider(p: unknown): boolean {
  return p === 2 || p === 'huawei';
}

export function providerToApiNumber(p: unknown): number {
  if (typeof p === 'number' && Number.isFinite(p)) return p;
  switch (p) {
    case 'ali':
      return 0;
    case 'tencent':
      return 1;
    case 'huawei':
      return 2;
    case 'aws':
      return 3;
    default:
      return Number(p) || 0;
  }
}
