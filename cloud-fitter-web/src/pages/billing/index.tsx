import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { connect, useModel } from 'umi';
import { Button, Card, DatePicker, Modal, Select, Space, Table, Typography, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import CloudAccountBar from '@/components/CloudAccountBar';
import { queryBillingBySystemId } from '@/services/billingBySystemId';
import { providerLabel } from '@/services/cloudConfig';
import { listSystems, SystemRow } from '@/services/systemManage';
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
  fetchByAccount: (p: { provider: number; accountName: string; billingMonth?: string }) => void;
  fetchBySystem: (p: { systemName: string; billingMonth?: string }) => void;
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
  const [systems, setSystems] = useState<SystemRow[]>([]);
  const [systemIdQuery, setSystemIdQuery] = useState<string | undefined>();
  const [bySystemModalOpen, setBySystemModalOpen] = useState(false);
  const [bySystemLoading, setBySystemLoading] = useState(false);
  const [bySystemPayload, setBySystemPayload] = useState<Awaited<
    ReturnType<typeof queryBillingBySystemId>
  > | null>(null);

  const billingMonthStr = useMemo(
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

  const loadSystems = useCallback(async () => {
    try {
      const res = await listSystems({ page: 1, pageSize: 500 });
      setSystems(res.systems ?? []);
    } catch (e: any) {
      message.error(e?.message || '加载系统列表失败');
    }
  }, []);

  useEffect(() => {
    void loadSystems();
  }, [loadSystems]);

  const onQueryBySystemId = async () => {
    const sid = (systemIdQuery ?? '').trim();
    if (!sid) {
      message.warning('请选择系统（按系统 ID 关联本地系统记录）');
      return;
    }
    setBySystemLoading(true);
    try {
      const data = await queryBillingBySystemId(sid, billingMonthStr);
      setBySystemPayload(data);
      setBySystemModalOpen(true);
    } catch (e: any) {
      const errText =
        e?.data?.error || e?.response?.data?.error || e?.message || '按系统查询账单失败';
      message.error(errText);
    } finally {
      setBySystemLoading(false);
    }
  };

  const accountModalColumns: ColumnsType<{
    accountName: string;
    provider: number;
    summary?: Record<string, unknown>;
  }> = [
    {
      title: '账号（account_name）',
      dataIndex: 'accountName',
      key: 'accountName',
      align: 'center',
    },
    {
      title: '云类型',
      dataIndex: 'provider',
      key: 'provider',
      align: 'center',
      render: (p: number) => PROVIDER_ENUM_CN[p] ?? providerLabel(p) ?? String(p),
    },
    {
      title: '消费总账',
      key: 'grand',
      align: 'right',
      render: (_: unknown, r) => {
        const g = r.summary?.grandTotalConsume;
        return g != null ? Number(g).toFixed(2) : '—';
      },
    },
    {
      title: '币种',
      key: 'cur',
      align: 'center',
      render: (_: unknown, r) => (r.summary?.currency as string) ?? '—',
    },
  ];

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
    {
      title: '账单月份',
      dataIndex: 'billingCycle',
      key: 'billingMonth',
      align: 'center',
    },
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
          fetchByAccount({ provider, accountName, billingMonth: billingMonthStr })
        }
        onQueryBySystem={(systemName) =>
          fetchBySystem({ systemName, billingMonth: billingMonthStr })
        }
        onClear={clearTable}
      />
      <Space style={{ marginBottom: 16 }} align="center">
        <span>账单月份：</span>
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
      <Card size="small" title="按系统 ID 查询（与 CMDB 分账号账单维度一致）" style={{ marginBottom: 16 }}>
        <Space wrap align="center">
          <span>系统：</span>
          <Select
            showSearch
            placeholder="选择系统（写入值为 systemId）"
            style={{ minWidth: 320 }}
            optionFilterProp="label"
            allowClear
            value={systemIdQuery}
            onChange={(v) => setSystemIdQuery(v)}
            options={systems.map((s) => ({
              label: `${s.name}（${s.systemId}）`,
              value: s.systemId,
            }))}
          />
          <Button type="primary" loading={bySystemLoading} onClick={() => void onQueryBySystemId()}>
            查询各账号账单
          </Button>
          <Text type="secondary">
            弹框中展示每个关联云账号（account_name）的账单汇总，对应 CMDB billing 同步字段。
          </Text>
        </Space>
      </Card>
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

      <Modal
        title={
          bySystemPayload
            ? `系统账单 — ${bySystemPayload.systemName}（${bySystemPayload.systemId}） ${bySystemPayload.billingMonth}`
            : '系统分账号账单'
        }
        open={bySystemModalOpen}
        onCancel={() => setBySystemModalOpen(false)}
        footer={null}
        width={960}
        destroyOnClose
      >
        <Table
          rowKey={(r) => r.accountName}
          dataSource={bySystemPayload?.accounts ?? []}
          columns={accountModalColumns}
          pagination={false}
          expandable={{
            expandedRowRender: (r) => {
              const rows = (r.summary?.rows as Record<string, unknown>[] | undefined) ?? [];
              return (
                <Table
                  size="small"
                  rowKey={(_, i) => String(i)}
                  dataSource={rows}
                  pagination={false}
                  columns={[
                    { title: '资源大类', dataIndex: 'category', key: 'category', align: 'center' },
                    {
                      title: '消费合计',
                      dataIndex: 'totalConsumeAmount',
                      key: 'totalConsumeAmount',
                      align: 'right',
                      render: (v: unknown) =>
                        v != null ? Number(v).toFixed(2) : '—',
                    },
                    { title: '币种', dataIndex: 'currency', key: 'currency', align: 'center' },
                    {
                      title: '汇总行数',
                      dataIndex: 'sourceRowCount',
                      key: 'sourceRowCount',
                      align: 'center',
                    },
                  ]}
                />
              );
            },
          }}
        />
      </Modal>
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
      billingMonth?: string;
    }) => ({
      type: 'billingPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: { systemName: string; billingMonth?: string }) => ({
      type: 'billingPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'billingPage/resetTable' }),
  },
)(BillingPage);
