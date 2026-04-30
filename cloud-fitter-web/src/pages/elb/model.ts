import { Effect, Reducer } from 'umi';
import { queryElbByAccount, queryElbBySystem } from './service';

export interface ElbPageState {
  tableData: any[];
  tableLoading: boolean;
}

export interface ElbPageModel {
  namespace: 'elbPage';
  state: ElbPageState;
  effects: {
    fetchByAccount: Effect;
    fetchBySystem: Effect;
  };
  reducers: {
    updateStore: Reducer<ElbPageState>;
    resetTable: Reducer<ElbPageState>;
  };
}

const model: ElbPageModel = {
  namespace: 'elbPage',
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
        const resp = yield call(queryElbByAccount, provider, accountName);
        const elbs = Array.isArray(resp?.elbs) ? resp.elbs : [];
        const tableData = elbs.map((item: any, index: number) =>
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
        const resp = yield call(queryElbBySystem, systemName);
        const elbs = Array.isArray(resp?.elbs) ? resp.elbs : [];
        const tableData = elbs.map((item: any, index: number) =>
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
