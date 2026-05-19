import { Effect, Reducer } from 'umi';
import { queryWafByAccount } from './service';

export interface WafPageState {
  tableData: any[];
  tableLoading: boolean;
}

export interface WafPageModel {
  namespace: 'wafPage';
  state: WafPageState;
  effects: {
    fetchByAccount: Effect;
  };
  reducers: {
    updateStore: Reducer<WafPageState>;
    resetTable: Reducer<WafPageState>;
  };
}

const model: WafPageModel = {
  namespace: 'wafPage',
  state: {
    tableData: [],
    tableLoading: false,
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      yield put({ type: 'updateStore', params: { tableLoading: true } });
      try {
        const { provider, accountName } = action.payload;
        const resp = yield call(queryWafByAccount, provider, accountName);
        const rows = Array.isArray(resp?.wafHosts) ? resp.wafHosts : [];
        const tableData = rows.map((item: any, index: number) =>
          Object.assign({}, item, { key: index }),
        );
        yield put({
          type: 'updateStore',
          params: { tableData },
        });
      } catch (_e) {
        yield put({
          type: 'updateStore',
          params: { tableData: [] },
        });
      } finally {
        yield put({ type: 'updateStore', params: { tableLoading: false } });
      }
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
      return { ...state, tableData: [], tableLoading: false };
    },
  },
};

export default model;
