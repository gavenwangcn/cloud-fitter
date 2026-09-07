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

/** 批量导出全部云账号在 [startMonth, endMonth] 的费用明细 xlsx */
export async function exportBillingBatch(startMonth: string, endMonth: string) {
  try {
    const data: any = await request('/apis/billing/batch-export', {
      method: 'POST',
      data: { startMonth, endMonth },
      responseType: 'blob',
      timeout: API_REQUEST_TIMEOUT_MS,
      skipErrorHandler: true,
    });
    const blob: Blob = data instanceof Blob ? data : new Blob([data]);
    if (isJsonBlob(blob)) {
      throw new Error(await messageFromJsonErrorBlob(blob));
    }
    const filename = `billing-export-${startMonth}-${endMonth}.xlsx`;
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
