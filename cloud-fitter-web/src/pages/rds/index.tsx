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
  clearTable: () => void;
}

const RdsPage: React.FC<RdsPageProps> = ({
  rdsPage,
  loading,
  fetchByAccount,
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
    loading: loading.effects['rdsPage/fetchByAccount'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'rdsPage/fetchByAccount',
      payload,
    }),
    clearTable: () => ({ type: 'rdsPage/resetTable' }),
  },
)(RdsPage);
