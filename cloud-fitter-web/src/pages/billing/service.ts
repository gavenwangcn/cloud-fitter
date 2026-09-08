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

export const BILLING_OTHER_DETAIL_CATEGORIES = [
  '文件存储',
  '云备份',
  '数据库复制',
  '数据库安全',
  'WAF',
  '云堡垒机',
  '云专线',
  '证书管理',
  'DNS',
  '其他',
] as const;

export type BillingOtherDetailCategory = (typeof BILLING_OTHER_DETAIL_CATEGORIES)[number];

export interface BillingOtherBreakdownRow {
  accountName: string;
  category: string;
  serviceTypeCode: string;
  consumeAmount: number;
  currency: string;
}

export interface BillingOtherBreakdownResp {
  rows: BillingOtherBreakdownRow[];
  total: number;
  currency: string;
}

/** 华为云「其他」行明细（按文件存储/云备份等大类拆分） */
export async function queryBillingOtherBreakdown(params: {
  provider: number;
  billingMonth: string;
  accountName?: string;
  systemName?: string;
}): Promise<BillingOtherBreakdownResp> {
  return request('/apis/billing/other-breakdown', {
    method: 'POST',
    data: params,
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
