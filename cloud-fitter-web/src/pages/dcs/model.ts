import { Effect, Reducer } from 'umi';
import { queryAllDcs } from './service';

export interface DcsPageState {
  tableData: any[];
}

export interface DcsPageModel {
  namespace: 'dcsPage';
  state: DcsPageState;
  effects: {
    fetchAll: Effect;
  };
  reducers: {
    updateStore: Reducer<DcsPageState>;
  };
}

const model: DcsPageModel = {
  namespace: 'dcsPage',
  state: {
    tableData: [],
  },
  effects: {
    *fetchAll(_, { call, put }) {
      const { redises = [] } = yield call(queryAllDcs);
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
  },
};

export default model;
