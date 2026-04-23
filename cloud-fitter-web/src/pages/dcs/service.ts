import { request } from 'umi';

/** DCS（华为等）走后端 Redis 聚合接口 */
export async function queryAllDcs() {
  return request('/apis/redis/all', {
    method: 'POST',
    data: {},
  });
}
