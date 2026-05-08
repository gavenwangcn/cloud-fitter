import React, { useEffect } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { connect } from 'umi';
import CloudAccountBar from '@/components/CloudAccountBar';
import FullResourceTable from '@/components/FullResourceTable';
import { CERTIFICATE_FIELDS } from '@/constants/resourceFields';
import { CertificatesPageState } from './model';

interface CertificatesPageProps {
  certificatesPage: CertificatesPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string }) => void;
  fetchBySystem: (p: { systemName: string }) => void;
  clearTable: () => void;
}

const CertificatesPage: React.FC<CertificatesPageProps> = ({
  certificatesPage,
  loading,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: '证书',
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
        resourceLabel="SSL 证书"
        fields={CERTIFICATE_FIELDS}
        dataSource={certificatesPage.tableData}
        loading={!!loading}
      />
    </div>
  );
};

export default connect(
  ({ certificatesPage, loading }: any) => ({
    certificatesPage,
    loading:
      loading.effects['certificatesPage/fetchByAccount'] ||
      loading.effects['certificatesPage/fetchBySystem'],
  }),
  {
    fetchByAccount: (payload: { provider: number; accountName: string }) => ({
      type: 'certificatesPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string }) => ({
      type: 'certificatesPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'certificatesPage/resetTable' }),
  },
)(CertificatesPage);
