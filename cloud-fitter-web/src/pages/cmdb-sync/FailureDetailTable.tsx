import React from 'react';
import { Table, Typography } from 'antd';
import { CmdbFailureDetail, CmdbResourceFailure } from '@/services/cmdbSync';

const { Text } = Typography;

function failureRows(rf?: CmdbResourceFailure): CmdbFailureDetail[] {
  if (rf?.failures?.length) {
    return rf.failures;
  }
  if (rf?.failed_ids?.length) {
    return rf.failed_ids.map((id) => ({ resource_id: id, reason: '—' }));
  }
  return [];
}

interface FailureDetailTableProps {
  resourceFailure?: CmdbResourceFailure;
  compact?: boolean;
}

/** 失败明细：资源 ID + 失败原因 */
export const FailureDetailTable: React.FC<FailureDetailTableProps> = ({
  resourceFailure,
  compact,
}) => {
  const rows = failureRows(resourceFailure);
  if (!rows.length) {
    return <Text type="secondary">无失败明细</Text>;
  }
  return (
    <Table
      rowKey={(r, i) => `${r.resource_id ?? ''}-${i}`}
      size="small"
      pagination={false}
      dataSource={rows}
      columns={[
        {
          title: '资源 ID',
          dataIndex: 'resource_id',
          width: compact ? 200 : 240,
          render: (v?: string) =>
            v ? (
              <Text code style={{ wordBreak: 'break-all' }}>
                {v}
              </Text>
            ) : (
              '—'
            ),
        },
        {
          title: '失败原因',
          dataIndex: 'reason',
          render: (v: string) => (
            <Text type="danger" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {v || '—'}
            </Text>
          ),
        },
      ]}
    />
  );
};

export default FailureDetailTable;
