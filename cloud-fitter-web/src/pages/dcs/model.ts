import { Effect, Reducer } from 'umi';
import { queryDcsByAccount, queryDcsBySystem } from './service';

export interface DcsPageState {
  tableData: any[];
}

export interface DcsPageModel {
  namespace: 'dcsPage';
  state: DcsPageState;
  effects: {
    fetchByAccount: Effect;
    fetchBySystem: Effect;
  };
  reducers: {
    updateStore: Reducer<DcsPageState>;
    resetTable: Reducer<DcsPageState>;
  };
}

const model: DcsPageModel = {
  namespace: 'dcsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { redises = [] } = yield call(queryDcsByAccount, provider, accountName);
      const tableData = redises.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
      yield put({
        type: 'updateStore',
        params: { tableData },
      });
    },
    *fetchBySystem(action: { payload: { systemName: string } }, { call, put }) {
      const { systemName } = action.payload;
      const { redises = [] } = yield call(queryDcsBySystem, systemName);
      const tableData = redises.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
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
