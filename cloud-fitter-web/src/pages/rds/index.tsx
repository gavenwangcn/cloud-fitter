import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import FullResourceTable from '@/components/FullResourceTable';
import { RDS_FIELDS } from '@/constants/resourceFields';
import { RdsPageState } from './model';

interface RdsPageProps {
  rdsPage: RdsPageState;
  loading?: boolean;
  fetchAll: () => void;
}

const RdsPage: React.FC<RdsPageProps> = ({ rdsPage, loading, fetchAll }) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'RDS',
    });
    fetchAll();
  }, []);

  return (
    <div className="pageContent">
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
    loading: loading.effects['rdsPage/fetchAll'],
  }),
  {
    fetchAll: () => ({ type: 'rdsPage/fetchAll' }),
  },
)(RdsPage);
