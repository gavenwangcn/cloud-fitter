import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import FullResourceTable from '@/components/FullResourceTable';
import { ECS_FIELDS } from '@/constants/resourceFields';
import { EcsPageState } from './model';

interface EcsPageProps {
  ecsPage: EcsPageState;
  loading?: boolean;
  fetchAll: () => void;
}

const EcsPage: React.FC<EcsPageProps> = ({ ecsPage, loading, fetchAll }) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'ECS',
    });
    fetchAll();
  }, []);

  return (
    <div className="pageContent">
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
    loading: loading.effects['ecsPage/fetchAll'],
  }),
  {
    fetchAll: () => ({ type: 'ecsPage/fetchAll' }),
  },
)(EcsPage);
