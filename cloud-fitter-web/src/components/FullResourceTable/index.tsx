import React from 'react';
import { Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  PROVIDER_FILTERS,
  ResourceFieldDef,
} from '@/constants/resourceFields';

function renderCell(val: unknown) {
  if (val === null || val === undefined || val === '') {
    return '—';
  }
  if (Array.isArray(val)) {
    if (val.length === 0) {
      return '—';
    }
    return (
      <div>
        {val.map((x, i) => (
          <div key={i}>{String(x)}</div>
        ))}
      </div>
    );
  }
  if (typeof val === 'object') {
    return JSON.stringify(val);
  }
  return String(val);
}

export interface FullResourceTableProps {
  resourceLabel: string;
  fields: ResourceFieldDef[];
  dataSource: any[];
  loading?: boolean;
}

const FullResourceTable: React.FC<FullResourceTableProps> = ({
  resourceLabel,
  fields,
  dataSource,
  loading,
}) => {
  const columns: ColumnsType<any> = [
    {
      title: '资源类型',
      key: '_resourceType',
      fixed: 'left',
      width: 100,
      align: 'center',
      render: () => resourceLabel,
    },
    ...fields.map((f) => ({
      title: f.title,
      dataIndex: f.dataIndex,
      key: f.dataIndex,
      align: 'center' as const,
      ellipsis: true,
      filters: f.filter ? PROVIDER_FILTERS : undefined,
      onFilter: f.filter
        ? (value: string | number | boolean, record: any) =>
            String(record[f.dataIndex] ?? '') === String(value)
        : undefined,
      render: (_: unknown, record: any) => renderCell(record[f.dataIndex]),
    })),
  ];

  return (
    <Table
      rowKey={(record, index) =>
        `${record.instanceId ?? 'row'}-${record.regionName ?? ''}-${index}`
      }
      loading={loading}
      dataSource={dataSource}
      columns={columns}
      pagination={false}
      scroll={{ x: 'max-content' }}
    />
  );
};

export default FullResourceTable;
