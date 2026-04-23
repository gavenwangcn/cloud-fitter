import { Effect, Reducer } from 'umi';
import { queryAllRds } from './service';

export interface RdsPageState {
  tableData: any[];
}

export interface RdsPageModel {
  namespace: 'rdsPage';
  state: RdsPageState;
  effects: {
    fetchAll: Effect;
  };
  reducers: {
    updateStore: Reducer<RdsPageState>;
  };
}

const model: RdsPageModel = {
  namespace: 'rdsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchAll(_, { call, put }) {
      const { rdses = [] } = yield call(queryAllRds);
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
  },
};

export default model;
