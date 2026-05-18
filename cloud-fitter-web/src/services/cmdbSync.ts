import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

export async function syncCmdbBySystemName(systemName: string): Promise<{ status?: string }> {
  return request('/apis/cmdb/sync', {
    method: 'POST',
    data: { systemName },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}
