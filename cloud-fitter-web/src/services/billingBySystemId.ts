import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { request } from 'umi';

/** 按 CMDB systemId 拉取本系统各关联云账号的账单汇总（与 CMDB 写入维度一致，含 account_name） */
export async function queryBillingBySystemId(systemId: string, billingMonth?: string) {
  return request<{
    systemId: string;
    systemName: string;
    billingMonth: string;
    accounts: Array<{
      accountName: string;
      provider: number;
      summary: {
        rows?: Array<Record<string, unknown>>;
        currency?: string;
        grandTotalConsume?: number;
      };
    }>;
  }>('/apis/billing/by-system-id', {
    method: 'POST',
    data: { systemId, billingMonth: billingMonth ?? '' },
    timeout: API_REQUEST_TIMEOUT_MS,
  });
}
