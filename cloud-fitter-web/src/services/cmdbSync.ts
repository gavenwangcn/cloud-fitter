import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

export async function syncCmdbBySystemName(systemName: string): Promise<{ status?: string }> {
  return request('/apis/cmdb/sync', {
    method: 'POST',
    data: { systemName },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

export type CmdbSyncTriggerType = 'scheduled' | 'manual';

export interface CmdbResourceFailure {
  fail_count: number;
  failed_ids?: string[];
}

export interface CmdbResourceFailBySystem {
  system_id: string;
  system_name: string;
  fail_count: number;
  failed_ids?: string[];
}

export interface CmdbResourceFailTotal {
  fail_count: number;
  items?: CmdbResourceFailBySystem[];
}

export interface CmdbSyncSystemDetail {
  system_id: string;
  system_name: string;
  status: string;
  error?: string;
  resources?: Record<string, CmdbResourceFailure>;
}

export interface CmdbSyncRunSummary {
  id: number;
  trigger_type: CmdbSyncTriggerType;
  manual_system_id?: string;
  manual_system_name?: string;
  started_at: string;
  finished_at?: string;
  total_systems: number;
  success_systems: number;
  failed_systems: number;
  skipped_systems: number;
  status: string;
  resource_fail_totals?: Record<string, CmdbResourceFailTotal>;
  systems?: CmdbSyncSystemDetail[];
}

export interface CmdbSyncRunListResp {
  total: number;
  items: CmdbSyncRunSummary[];
}

export async function listCmdbSyncRuns(params?: {
  page?: number;
  pageSize?: number;
}): Promise<CmdbSyncRunListResp> {
  return request('/apis/cmdb/sync/runs', {
    method: 'GET',
    params: {
      page: params?.page ?? 1,
      pageSize: params?.pageSize ?? 10,
    },
  });
}

export async function getCmdbSyncRun(id: number): Promise<CmdbSyncRunSummary> {
  return request(`/apis/cmdb/sync/runs/${id}`, {
    method: 'GET',
  });
}
