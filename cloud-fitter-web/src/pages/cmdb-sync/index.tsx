import React, { useCallback, useEffect, useState } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import {
  Alert,
  Button,
  Card,
  Form,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { listSystems, SystemRow } from '@/services/systemManage';
import {
  CmdbSyncRunSummary,
  listCmdbSyncRuns,
  syncCmdbBySystemName,
} from '@/services/cmdbSync';
import SyncLogDrawer from './SyncLogDrawer';

const { Paragraph, Text } = Typography;

const LOG_PAGE_SIZE = 10;

function formatTime(v?: string): string {
  if (!v) {
    return '—';
  }
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) {
    return v;
  }
  return d.toLocaleString('zh-CN', { hour12: false });
}

function statusTag(status?: string) {
  const s = (status || '').toLowerCase();
  if (s === 'success') {
    return <Tag color="success">成功</Tag>;
  }
  if (s === 'partial') {
    return <Tag color="warning">部分失败</Tag>;
  }
  if (s === 'failed') {
    return <Tag color="error">失败</Tag>;
  }
  if (s === 'running') {
    return <Tag color="processing">进行中</Tag>;
  }
  return <Tag>{status || '—'}</Tag>;
}

function triggerLabel(t?: string): string {
  if (t === 'manual') {
    return '手动';
  }
  if (t === 'scheduled') {
    return '定时';
  }
  return t || '—';
}

function resourceFailSummary(totals?: CmdbSyncRunSummary['resource_fail_totals']): string {
  if (!totals) {
    return '—';
  }
  const parts = Object.entries(totals)
    .filter(([, v]) => (v?.fail_count ?? 0) > 0)
    .map(([k, v]) => `${k}:${v.fail_count}`);
  return parts.length ? parts.join('；') : '—';
}

const CmdbSyncPage: React.FC = () => {
  const { setBreadcrumb } = useModel('layout');
  const [systems, setSystems] = useState<SystemRow[]>([]);
  const [loadingList, setLoadingList] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [form] = Form.useForm();

  const [logRows, setLogRows] = useState<CmdbSyncRunSummary[]>([]);
  const [logLoading, setLogLoading] = useState(false);
  const [logPage, setLogPage] = useState(1);
  const [logTotal, setLogTotal] = useState(0);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [selectedRun, setSelectedRun] = useState<CmdbSyncRunSummary | null>(null);

  const loadSystems = useCallback(async () => {
    setLoadingList(true);
    try {
      const res = await listSystems({ page: 1, pageSize: 500 });
      setSystems(res.systems ?? []);
    } catch (e: any) {
      message.error(e?.message || '加载系统列表失败');
    } finally {
      setLoadingList(false);
    }
  }, []);

  const loadLogs = useCallback(async (page: number) => {
    setLogLoading(true);
    try {
      const res = await listCmdbSyncRuns({ page, pageSize: LOG_PAGE_SIZE });
      setLogRows(res.items ?? []);
      setLogTotal(res.total ?? 0);
    } catch (e: any) {
      message.error(e?.data?.error || e?.message || '加载同步记录失败');
    } finally {
      setLogLoading(false);
    }
  }, []);

  useEffect(() => {
    setBreadcrumb({ isBack: false, title: 'CMDB 同步数据' });
  }, [setBreadcrumb]);

  useEffect(() => {
    void loadSystems();
  }, [loadSystems]);

  useEffect(() => {
    void loadLogs(logPage);
  }, [loadLogs, logPage]);

  const onSubmit = async () => {
    try {
      const v = await form.validateFields();
      const name = (v.systemName as string) || '';
      if (!name.trim()) {
        return;
      }
      setSyncing(true);
      try {
        await syncCmdbBySystemName(name.trim());
        message.success('该系统的 CMDB 全量同步已完成');
        setLogPage(1);
        void loadLogs(1);
      } catch (e: any) {
        const errText =
          e?.data?.error || e?.response?.data?.error || e?.message || '同步失败';
        message.error(errText);
        void loadLogs(logPage);
      } finally {
        setSyncing(false);
      }
    } catch {
      // validate only
    }
  };

  const openDetail = (row: CmdbSyncRunSummary) => {
    setSelectedRun(row);
    setDrawerOpen(true);
  };

  return (
    <div className="pageContent">
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="说明"
          description={
            <div>
              <Paragraph style={{ marginBottom: 8 }}>
                选择本平台的<strong>系统名称</strong>后，将先校验 <Text code>CMDB</Text>{' '}
                中是否存在与本地系统<strong>相同 system_id</strong> 的系统记录。若不存在，将提示「CMDB中没有相同系统信息」；若存在，则按与定时任务相同的规则，对<strong>该单一系统</strong>写入系统节点、K8S、主机、中间件、弹性公网 EIP、按账单月份（billing_month）与各云账号（account_name）的消费大类汇总（billing）等数据。
              </Paragraph>
              <Paragraph style={{ marginBottom: 0 }}>
                下方<strong>同步记录</strong>展示定时与手动同步的历史结果，可查看各资源失败总数及「系统 + 资源 ID」明细，便于排查问题（需后端启用 MySQL）。
              </Paragraph>
            </div>
          }
        />
        <Card title="单系统全量同步" style={{ maxWidth: 520 }}>
          <Form form={form} layout="vertical" onFinish={onSubmit}>
            <Form.Item
              name="systemName"
              label="系统名称"
              rules={[{ required: true, message: '请选择要同步的系统' }]}
            >
              <Select
                showSearch
                placeholder="请选择系统"
                loading={loadingList}
                optionFilterProp="label"
                options={systems.map((s) => ({
                  label: s.name,
                  value: s.name,
                }))}
                allowClear
              />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" loading={syncing}>
                开始同步
              </Button>
            </Form.Item>
          </Form>
        </Card>

        <Card
          title="同步记录"
          extra={
            <Button
              icon={<ReloadOutlined />}
              onClick={() => void loadLogs(logPage)}
              loading={logLoading}
            >
              刷新
            </Button>
          }
        >
          <Table<CmdbSyncRunSummary>
            rowKey="id"
            loading={logLoading}
            dataSource={logRows}
            pagination={{
              current: logPage,
              pageSize: LOG_PAGE_SIZE,
              total: logTotal,
              showSizeChanger: false,
              showTotal: (t) => `共 ${t} 条`,
              onChange: (p) => setLogPage(p),
            }}
            columns={[
              { title: 'ID', dataIndex: 'id', width: 72, align: 'center' },
              {
                title: '同步时间',
                dataIndex: 'started_at',
                width: 180,
                render: (v: string) => formatTime(v),
              },
              {
                title: '触发方式',
                dataIndex: 'trigger_type',
                width: 88,
                align: 'center',
                render: (v: string) => triggerLabel(v),
              },
              {
                title: '系统数',
                width: 72,
                align: 'center',
                render: (_, row) => row.total_systems,
              },
              {
                title: '成功',
                dataIndex: 'success_systems',
                width: 64,
                align: 'center',
              },
              {
                title: '失败',
                dataIndex: 'failed_systems',
                width: 64,
                align: 'center',
                render: (n: number) =>
                  n > 0 ? <Text type="danger">{n}</Text> : n,
              },
              {
                title: '跳过',
                dataIndex: 'skipped_systems',
                width: 64,
                align: 'center',
              },
              {
                title: '状态',
                dataIndex: 'status',
                width: 100,
                align: 'center',
                render: (s: string) => statusTag(s),
              },
              {
                title: '资源失败汇总',
                ellipsis: true,
                render: (_, row) => resourceFailSummary(row.resource_fail_totals),
              },
              {
                title: '操作',
                width: 88,
                align: 'center',
                fixed: 'right',
                render: (_, row) => (
                  <Button type="link" size="small" onClick={() => openDetail(row)}>
                    详情
                  </Button>
                ),
              },
            ]}
            scroll={{ x: 1100 }}
          />
        </Card>
      </Space>

      <SyncLogDrawer
        open={drawerOpen}
        runId={selectedRun?.id ?? null}
        preview={selectedRun}
        onClose={() => {
          setDrawerOpen(false);
          setSelectedRun(null);
        }}
      />
    </div>
  );
};

export default CmdbSyncPage;
