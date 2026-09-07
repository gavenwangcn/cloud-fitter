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
  downloadBillingBatchExportFile,
  listBillingBatchExportFiles,
  startBillingBatchExport,
} from './service';

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
  fetchByAccount: (p: {
    provider: number;
    accountName: string;
    startMonth?: string;
    endMonth?: string;
  }) => void;
  fetchBySystem: (p: { systemName: string; startMonth?: string; endMonth?: string }) => void;
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
  const [queryStart, setQueryStart] = useState<Dayjs | null>(() => dayjs());
  const [queryEnd, setQueryEnd] = useState<Dayjs | null>(() => dayjs());
  const [exportStart, setExportStart] = useState<Dayjs | null>(null);
  const [exportEnd, setExportEnd] = useState<Dayjs | null>(null);
  const [exporting, setExporting] = useState(false);
  const [exportFiles, setExportFiles] = useState<string[]>([]);
  const [selectedExportFile, setSelectedExportFile] = useState<string | undefined>();
  const [downloadingExport, setDownloadingExport] = useState(false);
  const [pendingExportFile, setPendingExportFile] = useState<string | null>(null);
  const [systems, setSystems] = useState<SystemRow[]>([]);
  const [systemIdQuery, setSystemIdQuery] = useState<string | undefined>();
  const [bySystemModalOpen, setBySystemModalOpen] = useState(false);
  const [bySystemLoading, setBySystemLoading] = useState(false);
  const [bySystemPayload, setBySystemPayload] = useState<Awaited<
    ReturnType<typeof queryBillingBySystemId>
  > | null>(null);

  const queryStartMonthStr = useMemo(
    () => (queryStart ? queryStart.format('YYYY-MM') : ''),
    [queryStart],
  );
  const queryEndMonthStr = useMemo(
    () => (queryEnd ? queryEnd.format('YYYY-MM') : ''),
    [queryEnd],
  );

  const canQueryRange =
    !!queryStart && !!queryEnd && !queryStart.isAfter(queryEnd, 'month');

  const queryMonthPayload = useMemo(
    () =>
      canQueryRange
        ? { startMonth: queryStartMonthStr, endMonth: queryEndMonthStr }
        : undefined,
    [canQueryRange, queryStartMonthStr, queryEndMonthStr],
  );

  const canExport =
    !!exportStart &&
    !!exportEnd &&
    !exportStart.isAfter(exportEnd, 'month');

  const onBatchExport = async () => {
    if (!canExport || !exportStart || !exportEnd) return;
    setExporting(true);
    try {
      const res = await startBillingBatchExport(
        exportStart.format('YYYY-MM'),
        exportEnd.format('YYYY-MM'),
      );
      const filename = res?.filename;
      if (filename) {
        setPendingExportFile(filename);
        setSelectedExportFile(filename);
      }
      message.info(res?.message || '导出任务已启动，完成后可下载');
      void loadExportFiles();
    } catch (e: any) {
      message.error(e?.message || '启动批量导出失败');
      setExporting(false);
      setPendingExportFile(null);
    }
  };

  const onDownloadExport = async () => {
    if (!selectedExportFile) {
      message.warning('请选择要下载的文件');
      return;
    }
    setDownloadingExport(true);
    try {
      await downloadBillingBatchExportFile(selectedExportFile);
      message.success('下载成功');
    } catch (e: any) {
      message.error(e?.message || '下载失败');
    } finally {
      setDownloadingExport(false);
    }
  };

  const loadExportFiles = useCallback(async () => {
    try {
      const res = await listBillingBatchExportFiles();
      setExportFiles(res?.files ?? []);
    } catch (e: any) {
      message.error(e?.message || '加载导出文件列表失败');
    }
  }, []);

  useEffect(() => {
    void loadExportFiles();
  }, [loadExportFiles]);

  useEffect(() => {
    if (!pendingExportFile) return undefined;
    const poll = async () => {
      try {
        const res = await listBillingBatchExportFiles();
        const files = res?.files ?? [];
        setExportFiles(files);
        if (files.includes(pendingExportFile)) {
          setPendingExportFile(null);
          setExporting(false);
          message.success('导出文件已生成，可点击下载');
        }
      } catch {
        // 轮询失败静默，避免刷屏
      }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 3000);
    return () => window.clearInterval(timer);
  }, [pendingExportFile]);

  useEffect(() => {
    setBreadcrumb({
      isBack: false,
      title: '账单汇总',
    });
  }, []);

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
      const data = await queryBillingBySystemId(sid, queryEndMonthStr || queryStartMonthStr);
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
      render: (_: unknown, __: any, index: number) => index + 1,
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
        onQuery={(provider, accountName) => {
          if (!queryMonthPayload) {
            message.warning('请选择有效的开始月份与结束月份');
            return;
          }
          fetchByAccount({ provider, accountName, ...queryMonthPayload });
        }}
        onQueryBySystem={(systemName) => {
          if (!queryMonthPayload) {
            message.warning('请选择有效的开始月份与结束月份');
            return;
          }
          fetchBySystem({ systemName, ...queryMonthPayload });
        }}
        onClear={clearTable}
        extra={
          <>
            <span>开始时间：</span>
            <DatePicker
              picker="month"
              value={exportStart}
              onChange={(d) => setExportStart(d)}
              allowClear
            />
            <span>结束时间：</span>
            <DatePicker
              picker="month"
              value={exportEnd}
              onChange={(d) => setExportEnd(d)}
              allowClear
            />
            <Button
              type="primary"
              disabled={!canExport}
              loading={exporting}
              onClick={() => void onBatchExport()}
            >
              导出
            </Button>
            <Select
              placeholder="选择导出文件"
              style={{ minWidth: 220 }}
              allowClear
              value={selectedExportFile}
              onChange={(v) => setSelectedExportFile(v)}
              options={exportFiles.map((f) => ({ label: f, value: f }))}
            />
            <Button
              disabled={!selectedExportFile}
              loading={downloadingExport}
              onClick={() => void onDownloadExport()}
            >
              下载
            </Button>
          </>
        }
      />
      <Space style={{ marginBottom: 16 }} align="center" wrap>
        <span>开始月份：</span>
        <DatePicker
          picker="month"
          value={queryStart}
          onChange={(d) => setQueryStart(d)}
          allowClear={false}
        />
        <span>结束月份：</span>
        <DatePicker
          picker="month"
          value={queryEnd}
          onChange={(d) => setQueryEnd(d)}
          allowClear={false}
        />
        <Text type="secondary">
          按起止月份汇总所选账号/系统在各产品大类的应付金额（含负数退费项）；明细按月份排序。华为云来自
          BSS 汇总账单，阿里云/腾讯云由账单明细聚合。
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
        rowKey={(r, i) => `${r.accountName}-${r.billingCycle}-${r.category}-${i}`}
        loading={!!loading}
        dataSource={billingPage.tableData}
        columns={columns}
        pagination={false}
        scroll={{ x: 'max-content', y: 480 }}
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
      startMonth?: string;
      endMonth?: string;
    }) => ({
      type: 'billingPage/fetchByAccount',
      payload,
    }),
    fetchBySystem: (payload: {
      systemName: string;
      startMonth?: string;
      endMonth?: string;
    }) => ({
      type: 'billingPage/fetchBySystem',
      payload,
    }),
    clearTable: () => ({ type: 'billingPage/resetTable' }),
  },
)(BillingPage);
