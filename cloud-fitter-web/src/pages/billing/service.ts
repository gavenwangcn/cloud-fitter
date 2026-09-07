import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

/** 按账号拉取大类消费汇总（账单月份默认由后端填当月，亦可显式传入 YYYY-MM） */
export async function queryBillingByAccount(
  provider: number,
  accountName: string,
  billingMonth?: string,
) {
  return request('/apis/billing/by-account', {
    method: 'POST',
    data: { provider, accountName, billingMonth: billingMonth ?? '' },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

export async function queryBillingBySystem(systemName: string, billingMonth?: string) {
  return request('/apis/billing/by-account', {
    method: 'POST',
    data: { systemName, billingMonth: billingMonth ?? '' },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

async function messageFromJsonErrorBlob(blob: Blob): Promise<string> {
  const text = await blob.text();
  let msg = '操作失败';
  try {
    const j = JSON.parse(text);
    msg = j.error || j.errMsg || msg;
  } catch {
    msg = text || msg;
  }
  return msg;
}

function isJsonBlob(blob: Blob): boolean {
  return !!blob.type && blob.type.includes('application/json');
}

function filenameFromContentDisposition(
  contentDisposition: string | undefined,
  fallback: string,
): string {
  if (!contentDisposition) return fallback;
  const quoted = /filename="([^"]+)"/i.exec(contentDisposition);
  if (quoted?.[1]) return quoted[1];
  const unquoted = /filename=([^;\s]+)/i.exec(contentDisposition);
  if (unquoted?.[1]) return unquoted[1].replace(/^"|"$/g, '');
  return fallback;
}

/** 启动异步批量导出（后台写 xlsx 到 batch-export 目录） */
export async function startBillingBatchExport(
  startMonth: string,
  endMonth: string,
): Promise<{ filename: string; message?: string }> {
  return request('/apis/billing/batch-export', {
    method: 'POST',
    data: { startMonth, endMonth },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

/** 列出已生成的批量导出文件 */
export async function listBillingBatchExportFiles(): Promise<{ files: string[] }> {
  return request('/apis/billing/batch-export/files', {
    method: 'GET',
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}

/** 下载选中的批量导出文件 */
export async function downloadBillingBatchExportFile(filename: string) {
  try {
    const result: any = await request('/apis/billing/batch-export/download', {
      method: 'GET',
      params: { filename },
      responseType: 'blob',
      timeout: API_REQUEST_TIMEOUT_MS,
      skipErrorHandler: true,
      getResponse: true,
    });
    const data = result?.data !== undefined ? result.data : result;
    const blob: Blob = data instanceof Blob ? data : new Blob([data]);
    if (isJsonBlob(blob)) {
      throw new Error(await messageFromJsonErrorBlob(blob));
    }
    const response = result?.response;
    const disposition =
      response?.headers && typeof response.headers.get === 'function'
        ? response.headers.get('content-disposition')
        : response?.headers?.['content-disposition'];
    const name = filenameFromContentDisposition(disposition ?? undefined, filename);
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    window.URL.revokeObjectURL(url);
  } catch (e: any) {
    if (e?.data instanceof Blob && isJsonBlob(e.data)) {
      throw new Error(await messageFromJsonErrorBlob(e.data));
    }
    throw e;
  }
}
