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
  clearTable: () => void;
}

const DmsPage: React.FC<DmsPageProps> = ({
  dmsPage,
  loading,
  fetchByAccount,
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
    loading: loading.effects['dmsPage/fetchByAccount'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'dmsPage/fetchByAccount',
      payload,
    }),
    clearTable: () => ({ type: 'dmsPage/resetTable' }),
  },
)(DmsPage);
