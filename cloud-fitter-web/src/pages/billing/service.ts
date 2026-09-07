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
  let msg = '导出失败';
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

/** 批量导出：全账号 × 多月份，耗时可远超普通 API */
const BILLING_BATCH_EXPORT_TIMEOUT_MS = 30 * 60 * 1000;

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

/** 批量导出全部云账号在 [startMonth, endMonth] 的费用明细 xlsx */
export async function exportBillingBatch(startMonth: string, endMonth: string) {
  const fallbackFilename = `billing-export-${startMonth}-${endMonth}.xlsx`;
  try {
    const result: any = await request('/apis/billing/batch-export', {
      method: 'POST',
      data: { startMonth, endMonth },
      responseType: 'blob',
      timeout: BILLING_BATCH_EXPORT_TIMEOUT_MS,
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
    const filename = filenameFromContentDisposition(
      disposition ?? undefined,
      fallbackFilename,
    );
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
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
