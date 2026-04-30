import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { ELB_FIELDS } from '@/constants/resourceFields';
import { ElbPageState } from './model';

interface ElbPageProps {
  elbPage: ElbPageState;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  fetchBySystem: (p: { systemName: string }) => void;
  clearTable: () => void;
}

const ElbPage: React.FC<ElbPageProps> = ({
  elbPage,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'ELB',
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
        resourceLabel="ELB"
        fields={ELB_FIELDS}
        dataSource={elbPage.tableData}
        loading={elbPage.tableLoading}
      />
    </div>
  );
};

export default connect(
  ({ elbPage }: any) => ({
    elbPage,
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'elbPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string }) => ({
      type: 'elbPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'elbPage/resetTable' }),
  },
)(ElbPage);
