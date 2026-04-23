import { request } from 'umi';

export async function queryAllRds() {
  return request('/apis/rds/all', {
    method: 'POST',
    data: {},
  });
}
