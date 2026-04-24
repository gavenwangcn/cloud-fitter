import { Effect, Reducer } from 'umi';
import { queryDmsByAccount, queryDmsBySystem } from './service';

export interface DmsPageState {
  tableData: any[];
}

export interface DmsPageModel {
  namespace: 'dmsPage';
  state: DmsPageState;
  effects: {
    fetchByAccount: Effect;
    fetchBySystem: Effect;
  };
  reducers: {
    updateStore: Reducer<DmsPageState>;
    resetTable: Reducer<DmsPageState>;
  };
}

const model: DmsPageModel = {
  namespace: 'dmsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { kafkas = [] } = yield call(queryDmsByAccount, provider, accountName);
      const tableData = kafkas.map((item: any, index: number) =>
        Object.assign({}, item, { key: index }),
      );
      yield put({
        type: 'updateStore',
        params: { tableData },
      });
    },
    *fetchBySystem(action: { payload: { systemName: string } }, { call, put }) {
      const { systemName } = action.payload;
      const { kafkas = [] } = yield call(queryDmsBySystem, systemName);
      const tableData = kafkas.map((item: any, index: number) =>
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
