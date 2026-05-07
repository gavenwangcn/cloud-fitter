import { Effect, Reducer } from 'umi';
import { queryBillingByAccount, queryBillingBySystem } from './service';

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
        payload: { provider: number; accountName: string; billingMonth?: string };
      },
      { call, put },
    ) {
      const { provider, accountName, billingMonth } = action.payload;
      const res = yield call(queryBillingByAccount, provider, accountName, billingMonth);
      const rows = res?.rows ?? [];
      const tableData = rows.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
      yield put({
        type: 'updateStore',
        params: {
          tableData,
          grandTotal: res?.grandTotalConsume ?? 0,
          currency: res?.currency ?? 'CNY',
        },
      });
    },
    *fetchBySystem(
      action: { payload: { systemName: string; billingMonth?: string } },
      { call, put },
    ) {
      const { systemName, billingMonth } = action.payload;
      const res = yield call(queryBillingBySystem, systemName, billingMonth);
      const rows = res?.rows ?? [];
      const tableData = rows.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
      yield put({
        type: 'updateStore',
        params: {
          tableData,
          grandTotal: res?.grandTotalConsume ?? 0,
          currency: res?.currency ?? 'CNY',
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
