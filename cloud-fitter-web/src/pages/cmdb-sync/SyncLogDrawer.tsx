import React, { useEffect, useState } from 'react';
import { Descriptions, Drawer, Spin, Table, Tag, Typography, message } from 'antd';
import {
  CmdbResourceFailTotal,
  CmdbSyncRunSummary,
  CmdbSyncSystemDetail,
  getCmdbSyncRun,
} from '@/services/cmdbSync';
import FailureDetailTable from './FailureDetailTable';

const { Text } = Typography;

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
  if (s === 'skipped') {
    return <Tag>跳过</Tag>;
  }
  if (s === 'error') {
    return <Tag color="error">异常</Tag>;
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

function resourceFailRows(totals?: Record<string, CmdbResourceFailTotal>) {
  if (!totals) {
    return [];
  }
  return Object.entries(totals)
    .filter(([, v]) => (v?.fail_count ?? 0) > 0)
    .map(([resourceType, v]) => ({
      key: resourceType,
      resourceType,
      failCount: v.fail_count,
      items: v.items ?? [],
    }));
}

interface SyncLogDrawerProps {
  open: boolean;
  runId: number | null;
  preview?: CmdbSyncRunSummary | null;
  onClose: () => void;
}

const SyncLogDrawer: React.FC<SyncLogDrawerProps> = ({ open, runId, preview, onClose }) => {
  const [loading, setLoading] = useState(false);
  const [detail, setDetail] = useState<CmdbSyncRunSummary | null>(null);

  useEffect(() => {
    if (!open || !runId) {
      setDetail(null);
      return;
    }
    setLoading(true);
    void getCmdbSyncRun(runId)
      .then((res) => setDetail(res))
      .catch((e: any) => {
        message.error(e?.data?.error || e?.message || '加载同步详情失败');
        setDetail(preview ?? null);
      })
      .finally(() => setLoading(false));
  }, [open, runId, preview]);

  const data = detail ?? preview;

  return (
    <Drawer
      title={data ? `同步记录 #${data.id}` : '同步记录详情'}
      width={920}
      open={open}
      onClose={onClose}
      destroyOnClose
    >
      {loading && !data ? (
        <Spin />
      ) : !data ? (
        <Text type="secondary">暂无数据</Text>
      ) : (
        <>
          <Descriptions bordered size="small" column={2} style={{ marginBottom: 16 }}>
            <Descriptions.Item label="同步时间">{formatTime(data.started_at)}</Descriptions.Item>
            <Descriptions.Item label="结束时间">{formatTime(data.finished_at)}</Descriptions.Item>
            <Descriptions.Item label="触发方式">{triggerLabel(data.trigger_type)}</Descriptions.Item>
            <Descriptions.Item label="批次状态">{statusTag(data.status)}</Descriptions.Item>
            <Descriptions.Item label="系统总数">{data.total_systems}</Descriptions.Item>
            <Descriptions.Item label="成功 / 失败 / 跳过">
              {data.success_systems} / {data.failed_systems} / {data.skipped_systems}
            </Descriptions.Item>
            {data.trigger_type === 'manual' && data.manual_system_name ? (
              <Descriptions.Item label="手动同步系统" span={2}>
                {data.manual_system_name}
                {data.manual_system_id ? (
                  <Text type="secondary" style={{ marginLeft: 8 }}>
                    ({data.manual_system_id})
                  </Text>
                ) : null}
              </Descriptions.Item>
            ) : null}
          </Descriptions>

          <Typography.Title level={5} style={{ marginTop: 8 }}>
            资源失败汇总
          </Typography.Title>
          <Table
            rowKey="key"
            size="small"
            pagination={false}
            dataSource={resourceFailRows(data.resource_fail_totals)}
            locale={{ emptyText: '本次同步无资源级失败' }}
            expandable={{
              expandedRowRender: (row) => (
                <Table
                  rowKey={(r) => `${r.system_id}-${r.system_name}`}
                  size="small"
                  pagination={false}
                  dataSource={row.items}
                  expandable={{
                    expandedRowRender: (item) => (
                      <FailureDetailTable
                        resourceFailure={{
                          fail_count: item.fail_count,
                          failed_ids: item.failed_ids,
                          failures: item.failures,
                        }}
                      />
                    ),
                    rowExpandable: (item) =>
                      Boolean(item.failures?.length || item.failed_ids?.length),
                  }}
                  columns={[
                    { title: '系统名称', dataIndex: 'system_name', width: 160 },
                    { title: 'system_id', dataIndex: 'system_id', width: 180 },
                    { title: '失败数', dataIndex: 'fail_count', width: 72, align: 'center' },
                  ]}
                />
              ),
              rowExpandable: (row) => row.items.length > 0,
            }}
            columns={[
              { title: '资源类型', dataIndex: 'resourceType', width: 120 },
              { title: '失败总数', dataIndex: 'failCount', width: 96, align: 'center' },
              {
                title: '涉及系统数',
                width: 110,
                align: 'center',
                render: (_, row) => row.items.length,
              },
            ]}
            style={{ marginBottom: 24 }}
          />

          <Typography.Title level={5}>各系统同步结果</Typography.Title>
          <Table<CmdbSyncSystemDetail>
            rowKey="system_id"
            size="small"
            pagination={false}
            dataSource={data.systems ?? []}
            locale={{ emptyText: '无系统明细' }}
            expandable={{
              expandedRowRender: (sys) => {
                const resources = sys.resources ?? {};
                const rows = Object.entries(resources).map(([type, rf]) => ({
                  key: type,
                  type,
                  rf,
                }));
                if (sys.error) {
                  return (
                    <div style={{ marginBottom: 8 }}>
                      <Text type="danger" style={{ whiteSpace: 'pre-wrap' }}>
                        {sys.error}
                      </Text>
                    </div>
                  );
                }
                if (!rows.length) {
                  return <Text type="secondary">无资源失败明细</Text>;
                }
                return (
                  <Table
                    rowKey="key"
                    size="small"
                    pagination={false}
                    dataSource={rows}
                    expandable={{
                      expandedRowRender: (row) => <FailureDetailTable resourceFailure={row.rf} compact />,
                      rowExpandable: (row) =>
                        Boolean(row.rf.failures?.length || row.rf.failed_ids?.length),
                    }}
                    columns={[
                      { title: '资源类型', dataIndex: 'type', width: 120 },
                      { title: '失败数', render: (_, row) => row.rf.fail_count, width: 80, align: 'center' },
                    ]}
                  />
                );
              },
              rowExpandable: (sys) =>
                Boolean(sys.error) || Boolean(sys.resources && Object.keys(sys.resources).length > 0),
            }}
            columns={[
              { title: '系统名称', dataIndex: 'system_name', width: 160 },
              { title: 'system_id', dataIndex: 'system_id', width: 180 },
              {
                title: '状态',
                dataIndex: 'status',
                width: 100,
                render: (s: string) => statusTag(s),
              },
              {
                title: '失败资源类型',
                render: (_, sys) => {
                  const keys = Object.keys(sys.resources ?? {});
                  return keys.length ? keys.join('、') : '—';
                },
              },
            ]}
          />
        </>
      )}
    </Drawer>
  );
};

export default SyncLogDrawer;
