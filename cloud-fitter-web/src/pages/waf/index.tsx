import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { WAF_FIELDS } from '@/constants/resourceFields';
import { WafPageState } from './model';

interface WafPageProps {
  wafPage: WafPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  clearTable: () => void;
}

const WafPage: React.FC<WafPageProps> = ({ wafPage, loading, fetchByAccount, clearTable }) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: 'WAF',
    });
  }, []);

  return (
    <div className="pageContent">
      <CloudAccountBar
        accountOnly
        onQuery={(provider, accountName) => fetchByAccount({ provider, accountName })}
        onClear={clearTable}
      />
      <FullResourceTable
        resourceLabel="WAF 防护域名"
        fields={WAF_FIELDS}
        dataSource={wafPage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ wafPage, loading }: any) => ({
    wafPage,
    loading: loading.effects['wafPage/fetchByAccount'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'wafPage/fetchByAccount',
      payload,
    }),
    clearTable: () => ({ type: 'wafPage/resetTable' }),
  },
)(WafPage);
