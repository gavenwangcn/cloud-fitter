import { Effect, Reducer } from 'umi';
import { monthRangeInclusive, sortBillingRows, sumConsumeAmount } from './monthRange';
import { queryBillingByAccount, queryBillingBySystem } from './service';
import dayjs from 'dayjs';

export interface BillingPageState {
  tableData: any[];
  grandTotal: number;
  currency: string;
}

export interface BillingPageModel {
  namespace: 'billingPage';
  state: BillingPageState;
  effects: {
    fetchByAccount: Effect;
    fetchBySystem: Effect;
  };
  reducers: {
    updateStore: Reducer<BillingPageState>;
    resetTable: Reducer<BillingPageState>;
  };
}

function parseMonthRange(startMonth?: string, endMonth?: string): string[] {
  const start = dayjs(startMonth, 'YYYY-MM', true);
  const end = dayjs(endMonth, 'YYYY-MM', true);
  if (!start.isValid() || !end.isValid()) {
    return [];
  }
  return monthRangeInclusive(start, end);
}

function rowsFromResponse(res: any): any[] {
  const rows = res?.rows ?? [];
  return rows.filter(
    (item: any) => item != null && item.totalConsumeAmount != null && item.totalConsumeAmount !== 0,
  );
}

const model: BillingPageModel = {
  namespace: 'billingPage',
  state: {
    tableData: [],
    grandTotal: 0,
    currency: 'CNY',
  },
  effects: {
    *fetchByAccount(
      action: {
        payload: {
          provider: number;
          accountName: string;
          startMonth?: string;
          endMonth?: string;
        };
      },
      { call, put },
    ) {
      const { provider, accountName, startMonth, endMonth } = action.payload;
      const months = parseMonthRange(startMonth, endMonth);
      if (months.length === 0) {
        yield put({
          type: 'updateStore',
          params: { tableData: [], grandTotal: 0, currency: 'CNY' },
        });
        return;
      }
      const merged: any[] = [];
      let currency = 'CNY';
      for (const billingMonth of months) {
        const res = yield call(queryBillingByAccount, provider, accountName, billingMonth);
        merged.push(...rowsFromResponse(res));
        if (res?.currency) {
          currency = res.currency;
        }
      }
      const sorted = sortBillingRows(merged);
      const tableData = sorted.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
      yield put({
        type: 'updateStore',
        params: {
          tableData,
          grandTotal: sumConsumeAmount(sorted),
          currency,
        },
      });
    },
    *fetchBySystem(
      action: {
        payload: { systemName: string; startMonth?: string; endMonth?: string };
      },
      { call, put },
    ) {
      const { systemName, startMonth, endMonth } = action.payload;
      const months = parseMonthRange(startMonth, endMonth);
      if (months.length === 0) {
        yield put({
          type: 'updateStore',
          params: { tableData: [], grandTotal: 0, currency: 'CNY' },
        });
        return;
      }
      const merged: any[] = [];
      let currency = 'CNY';
      for (const billingMonth of months) {
        const res = yield call(queryBillingBySystem, systemName, billingMonth);
        merged.push(...rowsFromResponse(res));
        if (res?.currency) {
          currency = res.currency;
        }
      }
      const sorted = sortBillingRows(merged);
      const tableData = sorted.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
      yield put({
        type: 'updateStore',
        params: {
          tableData,
          grandTotal: sumConsumeAmount(sorted),
          currency,
        },
      });
    },
  },
  reducers: {
    updateStore(state, { params }: any) {
      return {
        ...state,
        ...params,
      };
    },
    resetTable(state) {
      return {
        ...state,
        tableData: [],
        grandTotal: 0,
        currency: 'CNY',
      };
    },
  },
};

export default model;
