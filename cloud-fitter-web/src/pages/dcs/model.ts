import { Effect, Reducer } from 'umi';
import { queryDcsByAccount } from './service';

export interface DcsPageState {
  tableData: any[];
}

export interface DcsPageModel {
  namespace: 'dcsPage';
  state: DcsPageState;
  effects: {
    fetchByAccount: Effect;
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
