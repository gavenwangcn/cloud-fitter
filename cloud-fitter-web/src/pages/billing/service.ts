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
    timeout: 120000,
  });
}

export async function queryBillingBySystem(systemName: string, billingMonth?: string) {
  return request('/apis/billing/by-account', {
    method: 'POST',
    data: { systemName, billingMonth: billingMonth ?? '' },
    timeout: 120000,
  });
}
