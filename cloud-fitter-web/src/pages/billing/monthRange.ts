import dayjs, { Dayjs } from 'dayjs';

/** 闭区间 [start, end] 内每个 YYYY-MM（升序）。 */
export function monthRangeInclusive(start: Dayjs, end: Dayjs): string[] {
  if (start.isAfter(end, 'month')) {
    return [];
  }
  const out: string[] = [];
  let cur = start.startOf('month');
  const last = end.startOf('month');
  while (!cur.isAfter(last, 'month')) {
    out.push(cur.format('YYYY-MM'));
    cur = cur.add(1, 'month');
  }
  return out;
}

export function sortBillingRows<T extends {
  billingCycle?: string;
  accountName?: string;
  category?: string;
}>(rows: T[]): T[] {
  return [...rows].sort((a, b) => {
    const mc = (a.billingCycle ?? '').localeCompare(b.billingCycle ?? '');
    if (mc !== 0) return mc;
    const ac = (a.accountName ?? '').localeCompare(b.accountName ?? '');
    if (ac !== 0) return ac;
    return (a.category ?? '').localeCompare(b.category ?? '');
  });
}

export function sumConsumeAmount(rows: { totalConsumeAmount?: number | null }[]): number {
  return rows.reduce((s, r) => s + (Number(r.totalConsumeAmount) || 0), 0);
}
