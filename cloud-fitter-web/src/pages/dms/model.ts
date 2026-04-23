import { Effect, Reducer } from 'umi';
import { queryDmsByAccount } from './service';

export interface DmsPageState {
  tableData: any[];
}

export interface DmsPageModel {
  namespace: 'dmsPage';
  state: DmsPageState;
  effects: {
    fetchByAccount: Effect;
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
