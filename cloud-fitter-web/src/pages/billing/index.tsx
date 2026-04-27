import React, { useEffect, useMemo, useState } from 'react';
import { connect, useModel } from 'umi';
import { Card, DatePicker, Space, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import CloudAccountBar from '@/components/CloudAccountBar';
import { providerLabel } from '@/services/cloudConfig';
import { BillingPageState } from './model';
import {
  RESOURCE_TABLE_DEFAULT_PAGE_SIZE,
  RESOURCE_TABLE_PAGE_SIZE_OPTIONS,
} from '@/constants/tablePagination';

const { Text } = Typography;

const PROVIDER_ENUM_CN: Record<number, string> = {
  0: '阿里云',
  1: '腾讯云',
  2: '华为云',
  3: 'AWS',
};

interface BillingPageProps {
  billingPage: BillingPageState;
  loading?: boolean;
  fetchByAccount: (p: { provider: number; accountName: string; billingCycle?: string }) => void;
  fetchBySystem: (p: { systemName: string; billingCycle?: string }) => void;
  clearTable: () => void;
}

const BillingPage: React.FC<BillingPageProps> = ({
  billingPage,
  loading,
  fetchByAccount,
  fetchBySystem,
  clearTable,
}) => {
  const { setBreadcrumb } = useModel('layout');
  const [month, setMonth] = useState<Dayjs | null>(() => dayjs());
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(RESOURCE_TABLE_DEFAULT_PAGE_SIZE);

  const billingCycleStr = useMemo(
    () => (month ? month.format('YYYY-MM') : ''),
    [month],
  );

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: '账单汇总',
    });
  }, []);

  useEffect(() => {
    setPage(1);
  }, [billingPage.tableData]);

  const columns: ColumnsType<any> = [
    {
      title: '序号',
      key: '_index',
      width: 72,
      align: 'center',
      render: (_: unknown, __: any, index: number) =>
        (page - 1) * pageSize + index + 1,
    },
    {
      title: '云类型',
      dataIndex: 'provider',
      key: 'provider',
      align: 'center',
      render: (p: number) => PROVIDER_ENUM_CN[p] ?? providerLabel(p) ?? String(p),
    },
    { title: '账号/范围', dataIndex: 'accountName', key: 'accountName', align: 'center' },
    { title: '账期', dataIndex: 'billingCycle', key: 'billingCycle', align: 'center' },
    { title: '资源大类', dataIndex: 'category', key: 'category', align: 'center' },
    {
      title: '消费合计',
      dataIndex: 'totalConsumeAmount',
      key: 'totalConsumeAmount',
      align: 'right',
      render: (v: number) => (v != null ? Number(v).toFixed(2) : '—'),
    },
    { title: '币种', dataIndex: 'currency', key: 'currency', align: 'center' },
    { title: '汇总行数', dataIndex: 'sourceRowCount', key: 'sourceRowCount', align: 'center' },
  ];

  return (
    <div className="pageContent">
      <CloudAccountBar
        onQuery={(provider, accountName) =>
          fetchByAccount({ provider, accountName, billingCycle: billingCycleStr })
        }
        onQueryBySystem={(systemName) =>
          fetchBySystem({ systemName, billingCycle: billingCycleStr })
        }
        onClear={clearTable}
      />
      <Space style={{ marginBottom: 16 }} align="center">
        <span>账期：</span>
        <DatePicker
          picker="month"
          value={month}
          onChange={(d) => setMonth(d)}
          allowClear={false}
        />
        <Text type="secondary">
          所选账号/系统在各产品大类（ECS、RDS、DCS 等）的应付金额汇总；华为云来自 BSS
          汇总账单，阿里云/腾讯云由账单明细聚合。需具备账单只读权限。
        </Text>
      </Space>
      <Table
        rowKey={(r) => `${r.key}-${r.category}-${r.accountName}`}
        loading={!!loading}
        dataSource={billingPage.tableData}
        columns={columns}
        pagination={{
          current: page,
          pageSize,
          total: billingPage.tableData.length,
          showTotal: (t) => `共 ${t} 条`,
          showSizeChanger: true,
          pageSizeOptions: [...RESOURCE_TABLE_PAGE_SIZE_OPTIONS],
          onChange: (p, ps) => {
            setPage(p);
            if (ps) {
              setPageSize(ps);
            }
          },
        }}
        scroll={{ x: 'max-content' }}
      />
      <Card size="small" style={{ marginTop: 16 }} title="消费总账">
        <Text strong>
          {billingPage.grandTotal != null ? Number(billingPage.grandTotal).toFixed(2) : '—'}{' '}
          {billingPage.currency || 'CNY'}
        </Text>
      </Card>
    </div>
  );
};

export default connect(
  ({ billingPage, loading }: any) => ({
    billingPage,
    loading:
      loading.effects['billingPage/fetchByAccount'] || loading.effects['billingPage/fetchBySystem'],
  }),
  {
    fetchByAccount: (payload: {
      provider: number;
      accountName: string;
      billingCycle?: string;
    }) => ({
      type: 'billingPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string; billingCycle?: string }) => ({
      type: 'billingPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'billingPage/resetTable' }),
  },
)(BillingPage);
