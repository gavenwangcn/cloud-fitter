import { request } from 'umi';

/** 从服务端 data 目录序号文件分配下一个系统ID（YH-000001 / D-000001） */
export async function allocateNextSystemId(kind: 'YH' | 'D'): Promise<{ systemId: string }> {
  return request('/apis/system-id/next', {
    method: 'POST',
    data: { kind },
  });
}
