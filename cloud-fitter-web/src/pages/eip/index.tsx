import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { EIP_FIELDS } from '@/constants/resourceFields';
import { EipPageState } from './model';

interface EipPageProps {
  eipPage: EipPageState;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  fetchBySystem: (p: { systemName: string }) => void;
  clearTable: () => void;
}

const EipPage: React.FC<EipPageProps> = ({
  eipPage,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'EIP',
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
        resourceLabel="EIP"
        fields={EIP_FIELDS}
        dataSource={eipPage.tableData}
        loading={eipPage.tableLoading}
      />
    </div>
  );
};

export default connect(
  ({ eipPage }: any) => ({
    eipPage,
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'eipPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string }) => ({
      type: 'eipPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'eipPage/resetTable' }),
  },
)(EipPage);
