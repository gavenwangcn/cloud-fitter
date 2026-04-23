import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { CCE_FIELDS } from '@/constants/resourceFields';
import { CcePageState } from './model';

interface CcePageProps {
  ccePage: CcePageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  clearTable: () => void;
}

const CcePage: React.FC<CcePageProps> = ({
  ccePage,
  loading,
  fetchByAccount,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'CCE',
    });
  }, []);

  return (
    <div className="pageContent">
      <CloudAccountBar
        onQuery={(provider, accountName) => fetchByAccount({ provider, accountName })}
        onClear={clearTable}
      />
      <FullResourceTable
        resourceLabel="CCE"
        fields={CCE_FIELDS}
        dataSource={ccePage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ ccePage, loading }: any) => ({
    ccePage,
    loading: loading.effects['ccePage/fetchByAccount'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'ccePage/fetchByAccount',
      payload,
    }),
    clearTable: () => ({ type: 'ccePage/resetTable' }),
  },
)(CcePage);
