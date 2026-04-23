import { request } from 'umi';

export async function queryAllEcs() {
  return request('/apis/ecs/all', {
    method: 'POST',
    data: {},
  });
}
