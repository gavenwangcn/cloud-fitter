import { Effect, Reducer } from 'umi';
import { queryCceByAccount } from './service';

export interface CcePageState {
  tableData: any[];
}

export interface CcePageModel {
  namespace: 'ccePage';
  state: CcePageState;
  effects: {
    fetchByAccount: Effect;
  };
  reducers: {
    updateStore: Reducer<CcePageState>;
    resetTable: Reducer<CcePageState>;
  };
}

const model: CcePageModel = {
  namespace: 'ccePage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { clusters = [] } = yield call(queryCceByAccount, provider, accountName);
      const tableData = clusters.map((item: any, index: number) =>
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
