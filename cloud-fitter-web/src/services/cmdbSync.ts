import { request } from 'umi';

export async function syncCmdbBySystemName(systemName: string): Promise<{ status?: string }> {
  return request('/apis/cmdb/sync', {
    method: 'POST',
    data: { systemName },
  });
}
