import { Effect, Reducer } from 'umi';
import { queryCertificatesByAccount, queryCertificatesBySystem } from './service';

export interface CertificatesPageState {
  tableData: any[];
  tableLoading: boolean;
}

export interface CertificatesPageModel {
  namespace: 'certificatesPage';
  state: CertificatesPageState;
  effects: {
    fetchByAccount: Effect;
    fetchBySystem: Effect;
  };
  reducers: {
    updateStore: Reducer<CertificatesPageState>;
    resetTable: Reducer<CertificatesPageState>;
  };
}

const model: CertificatesPageModel = {
  namespace: 'certificatesPage',
  state: {
    tableData: [],
    tableLoading: false,
  },
  effects: {
    *fetchByAccount(
      action: { payload: { provider: number; accountName: string } },
      { call, put },
    ) {
      yield put({ type: 'updateStore', params: { tableLoading: true } });
      try {
        const { provider, accountName } = action.payload;
        const resp = yield call(queryCertificatesByAccount, provider, accountName);
        const rows = Array.isArray(resp?.certificates) ? resp.certificates : [];
        const tableData = rows.map((item: any, index: number) =>
          Object.assign({}, item, { key: index }),
        );
        yield put({
          type: 'updateStore',
          params: { tableData },
        });
      } catch (_e) {
        yield put({
          type: 'updateStore',
          params: { tableData: [] },
        });
      } finally {
        yield put({ type: 'updateStore', params: { tableLoading: false } });
      }
    },
    *fetchBySystem(action: { payload: { systemName: string } }, { call, put }) {
      yield put({ type: 'updateStore', params: { tableLoading: true } });
      try {
        const { systemName } = action.payload;
        const resp = yield call(queryCertificatesBySystem, systemName);
        const rows = Array.isArray(resp?.certificates) ? resp.certificates : [];
        const tableData = rows.map((item: any, index: number) =>
          Object.assign({}, item, { key: index }),
        );
        yield put({
          type: 'updateStore',
          params: { tableData },
        });
      } catch (_e) {
        yield put({
          type: 'updateStore',
          params: { tableData: [] },
        });
      } finally {
        yield put({ type: 'updateStore', params: { tableLoading: false } });
      }
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
      return { ...state, tableData: [], tableLoading: false };
    },
  },
};

export default model;
