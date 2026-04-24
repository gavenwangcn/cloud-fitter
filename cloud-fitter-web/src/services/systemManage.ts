import { request } from 'umi';

export interface SystemRow {
  id: number;
  name: string;
  intro: string;
  systemId: string;
  accountIds: number[];
  accountNames: string[];
}

export async function listSystems(): Promise<{ systems: SystemRow[] }> {
  return request('/apis/systems', { method: 'GET' });
}

export async function createSystem(data: {
  name: string;
  intro: string;
  systemId: string;
  accountIds: number[];
}) {
  return request('/apis/systems', { method: 'POST', data });
}
