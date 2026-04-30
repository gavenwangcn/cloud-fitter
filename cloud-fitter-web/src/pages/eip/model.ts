import { Effect, Reducer } from 'umi';
import { queryEipByAccount, queryEipBySystem } from './service';

export interface EipPageState {
  tableData: any[];
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
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      const { provider, accountName } = action.payload;
      const { eips = [] } = yield call(queryEipByAccount, provider, accountName);
      const tableData = eips.map((item: any, index: number) => Object.assign({}, item, { key: index }));
      yield put({
        type: 'updateStore',
        params: { tableData },
      });
    },
    *fetchBySystem(action: { payload: { systemName: string } }, { call, put }) {
      const { systemName } = action.payload;
      const { eips = [] } = yield call(queryEipBySystem, systemName);
      const tableData = eips.map((item: any, index: number) => Object.assign({}, item, { key: index }));
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
