import { Effect, Reducer } from 'umi';
import { queryEcsByAccount } from './service';

export interface EcsPageState {
  tableData: any[];
}

export interface EcsPageModel {
  namespace: 'ecsPage';
  state: EcsPageState;
  effects: {
    fetchByAccount: Effect;
  };
  reducers: {
    updateStore: Reducer<EcsPageState>;
    resetTable: Reducer<EcsPageState>;
  };
}

const model: EcsPageModel = {
  namespace: 'ecsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { ecses = [] } = yield call(queryEcsByAccount, provider, accountName);
      const tableData = ecses.map((item: any, index: number) =>
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
