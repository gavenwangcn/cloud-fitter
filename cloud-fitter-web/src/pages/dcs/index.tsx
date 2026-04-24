import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { DCS_FIELDS } from '@/constants/resourceFields';
import { DcsPageState } from './model';

interface DcsPageProps {
  dcsPage: DcsPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  fetchBySystem: (p: { systemName: string }) => void;
  clearTable: () => void;
}

const DcsPage: React.FC<DcsPageProps> = ({
  dcsPage,
  loading,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'DCS',
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
        resourceLabel="DCS"
        fields={DCS_FIELDS}
        dataSource={dcsPage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ dcsPage, loading }: any) => ({
    dcsPage,
    loading:
      loading.effects['dcsPage/fetchByAccount'] || loading.effects['dcsPage/fetchBySystem'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'dcsPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string }) => ({
      type: 'dcsPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'dcsPage/resetTable' }),
  },
)(DcsPage);
