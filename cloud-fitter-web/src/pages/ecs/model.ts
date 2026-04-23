import { Effect, Reducer } from 'umi';
import { queryAllEcs } from './service';

export interface EcsPageState {
  tableData: any[];
}

export interface EcsPageModel {
  namespace: 'ecsPage';
  state: EcsPageState;
  effects: {
    fetchAll: Effect;
  };
  reducers: {
    updateStore: Reducer<EcsPageState>;
  };
}

const model: EcsPageModel = {
  namespace: 'ecsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchAll(_, { call, put }) {
      const { ecses = [] } = yield call(queryAllEcs);
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
  },
};

export default model;
