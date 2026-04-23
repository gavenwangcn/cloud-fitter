import { Effect, Reducer } from 'umi';
import { queryRdsByAccount } from './service';

export interface RdsPageState {
  tableData: any[];
}

export interface RdsPageModel {
  namespace: 'rdsPage';
  state: RdsPageState;
  effects: {
    fetchByAccount: Effect;
  };
  reducers: {
    updateStore: Reducer<RdsPageState>;
    resetTable: Reducer<RdsPageState>;
  };
}

const model: RdsPageModel = {
  namespace: 'rdsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { rdses = [] } = yield call(queryRdsByAccount, provider, accountName);
      const tableData = rdses.map((item: any, index: number) =>
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
