import { Effect, Reducer } from 'umi';
import { queryElbByAccount, queryElbBySystem } from './service';

export interface ElbPageState {
  tableData: any[];
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
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { elbs = [] } = yield call(queryElbByAccount, provider, accountName);
      const tableData = elbs.map((item: any, index: number) => Object.assign({}, item, { key: index }));
      yield put({
        type: 'updateStore',
        params: { tableData },
      });
    },
    *fetchBySystem(action: { payload: { systemName: string } }, { call, put }) {
      const { systemName } = action.payload;
      const { elbs = [] } = yield call(queryElbBySystem, systemName);
      const tableData = elbs.map((item: any, index: number) => Object.assign({}, item, { key: index }));
      yield put({
        type: 'updateStore',
        params: { tableData },
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
      return { ...state, tableData: [] };
    },
  },
};

export default model;
