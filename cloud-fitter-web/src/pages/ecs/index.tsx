import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { ECS_FIELDS } from '@/constants/resourceFields';
import { EcsPageState } from './model';

interface EcsPageProps {
  ecsPage: EcsPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  clearTable: () => void;
}

const EcsPage: React.FC<EcsPageProps> = ({
  ecsPage,
  loading,
  fetchByAccount,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'ECS',
    });
  }, []);

  return (
    <div className="pageContent">
      <CloudAccountBar
        onQuery={(provider, accountName) => fetchByAccount({ provider, accountName })}
        onClear={clearTable}
      />
      <FullResourceTable
        resourceLabel="ECS"
        fields={ECS_FIELDS}
        dataSource={ecsPage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ ecsPage, loading }: any) => ({
    ecsPage,
    loading: loading.effects['ecsPage/fetchByAccount'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'ecsPage/fetchByAccount',
      payload,
    }),
    clearTable: () => ({ type: 'ecsPage/resetTable' }),
  },
)(EcsPage);
