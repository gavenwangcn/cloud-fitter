import { request } from 'umi';

export interface SystemRow {
  id: number;
  name: string;
  intro: string;
  systemId: string;
  onlineTime: string;
  status: string;
  accountIds: number[];
  accountNames: string[];
}

export async function listSystems(params?: {
  page?: number;
  pageSize?: number;
}): Promise<{ systems: SystemRow[]; total?: number }> {
  return request('/apis/systems', { method: 'GET', params: params || {} });
}

export async function createSystem(data: {
  name: string;
  intro: string;
  systemId: string;
  onlineTime: string;
  status: string;
  accountIds: number[];
}) {
  return request('/apis/systems', { method: 'POST', data });
}

export async function updateSystem(data: {
  id: number;
  systemId: string;
  intro: string;
  onlineTime: string;
  status: string;
  accountIds: number[];
}) {
  return request('/apis/systems', { method: 'PUT', data });
}

export async function deleteSystem(id: number) {
  return request('/apis/systems', { method: 'DELETE', params: { id } });
}
