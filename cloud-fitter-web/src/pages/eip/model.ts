import { Effect, Reducer } from 'umi';
import { queryEipByAccount, queryEipBySystem } from './service';

export interface EipPageState {
  tableData: any[];
  tableLoading: boolean;
}

export interface EipPageModel {
  namespace: 'eipPage';
  state: EipPageState;
  effects: {
    fetchByAccount: Effect;
    fetchBySystem: Effect;
  };
  reducers: {
    updateStore: Reducer<EipPageState>;
    resetTable: Reducer<EipPageState>;
  };
}

const model: EipPageModel = {
  namespace: 'eipPage',
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
        const resp = yield call(queryEipByAccount, provider, accountName);
        const eips = Array.isArray(resp?.eips) ? resp.eips : [];
        const tableData = eips.map((item: any, index: number) =>
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
    *fetchBySystem(action: { payload: { systemName: string } }, { call, put }) {
      yield put({ type: 'updateStore', params: { tableLoading: true } });
      try {
        const { systemName } = action.payload;
        const resp = yield call(queryEipBySystem, systemName);
        const eips = Array.isArray(resp?.eips) ? resp.eips : [];
        const tableData = eips.map((item: any, index: number) =>
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
