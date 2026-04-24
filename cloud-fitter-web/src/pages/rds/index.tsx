import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { RDS_FIELDS } from '@/constants/resourceFields';
import { RdsPageState } from './model';

interface RdsPageProps {
  rdsPage: RdsPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  fetchBySystem: (p: { systemName: string }) => void;
  clearTable: () => void;
}

const RdsPage: React.FC<RdsPageProps> = ({
  rdsPage,
  loading,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'RDS',
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
        resourceLabel="RDS"
        fields={RDS_FIELDS}
        dataSource={rdsPage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ rdsPage, loading }: any) => ({
    rdsPage,
    loading:
      loading.effects['rdsPage/fetchByAccount'] || loading.effects['rdsPage/fetchBySystem'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'rdsPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string }) => ({
      type: 'rdsPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'rdsPage/resetTable' }),
  },
)(RdsPage);
