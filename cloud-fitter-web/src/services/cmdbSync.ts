import { request } from 'umi';

/** CMDB 手工同步可能持续数分钟，避免沿用全局短超时（默认 20s）误报失败 */
const CMDB_SYNC_TIMEOUT_MS = 5 * 60 * 1000;

export async function syncCmdbBySystemName(systemName: string): Promise<{ status?: string }> {
  return request('/apis/cmdb/sync', {
    method: 'POST',
    data: { systemName },
    timeout: CMDB_SYNC_TIMEOUT_MS,
  });
}
