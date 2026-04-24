import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { DMS_FIELDS } from '@/constants/resourceFields';
import { DmsPageState } from './model';

interface DmsPageProps {
  dmsPage: DmsPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  fetchBySystem: (p: { systemName: string }) => void;
  clearTable: () => void;
}

const DmsPage: React.FC<DmsPageProps> = ({
  dmsPage,
  loading,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'DMS',
    });
  }, []);

  return (
    <div className="pageContent">
      <CloudAccountBar
        onQuery={(provider, accountName) => fetchByAccount({ provider, accountName })}
        onQueryBySystem={(systemName) => fetchBySystem({ systemName })}
        onClear={clearTable}
      />
      <FullResourceTable
        resourceLabel="DMS"
        fields={DMS_FIELDS}
        dataSource={dmsPage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ dmsPage, loading }: any) => ({
    dmsPage,
    loading:
      loading.effects['dmsPage/fetchByAccount'] || loading.effects['dmsPage/fetchBySystem'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'dmsPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string }) => ({
      type: 'dmsPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'dmsPage/resetTable' }),
  },
)(DmsPage);
