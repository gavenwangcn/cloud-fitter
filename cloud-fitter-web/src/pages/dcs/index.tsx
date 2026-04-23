import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import FullResourceTable from '@/components/FullResourceTable';
import { DCS_FIELDS } from '@/constants/resourceFields';
import { DcsPageState } from './model';

interface DcsPageProps {
  dcsPage: DcsPageState;
  loading?: boolean;
  fetchAll: () => void;
}

const DcsPage: React.FC<DcsPageProps> = ({ dcsPage, loading, fetchAll }) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'DCS',
    });
    fetchAll();
  }, []);

  return (
    <div className="pageContent">
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
    loading: loading.effects['dcsPage/fetchAll'],
  }),
  {
    fetchAll: () => ({ type: 'dcsPage/fetchAll' }),
  },
)(DcsPage);
