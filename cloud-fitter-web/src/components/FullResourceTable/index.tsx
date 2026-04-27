import React, { useEffect, useState } from 'react';
import { Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  PROVIDER_FILTERS,
  ResourceFieldDef,
} from '@/constants/resourceFields';
import {
  RESOURCE_TABLE_DEFAULT_PAGE_SIZE,
  RESOURCE_TABLE_PAGE_SIZE_OPTIONS,
} from '@/constants/tablePagination';

/** 与后端 cloudTypeLabel 一致，用于无「节点」标签时展示 云名-地域 */
const PROVIDER_ENUM_CN: Record<number, string> = {
  0: '阿里云',
  1: '腾讯云',
  2: '华为云',
  3: 'AWS',
};

function effectiveNodeLabel(record: any): string {
  const raw = record?.nodeTagValue;
  if (raw !== null && raw !== undefined && String(raw).trim() !== '') {
    return String(raw);
  }
  const pv = record?.provider;
  const p =
    typeof pv === 'number'
      ? PROVIDER_ENUM_CN[pv] ?? '云'
      : { ali: '阿里云', tencent: '腾讯云', huawei: '华为云', aws: 'AWS' }[
          String(pv)
        ] ?? '云';
  const r = String(record?.regionName ?? '').trim();
  if (!r) {
    return '—';
  }
  return `${p}-${r}`;
}

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
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(RESOURCE_TABLE_DEFAULT_PAGE_SIZE);

  useEffect(() => {
    setPage(1);
  }, [dataSource]);

  const columns: ColumnsType<any> = [
    {
      title: '序号',
      key: '_index',
      fixed: 'left',
      width: 72,
      align: 'center',
      render: (_: unknown, __: any, index: number) =>
        (page - 1) * pageSize + index + 1,
    },
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
      render: (_: unknown, record: any) =>
        f.dataIndex === 'nodeTagValue'
          ? renderCell(effectiveNodeLabel(record))
          : renderCell(record[f.dataIndex]),
    })),
  ];

  return (
    <Table
      rowKey={(record, index) =>
        `${record.clusterUid ?? record.instanceId ?? 'row'}-${
          record.regionName ?? ''
        }-${index}`
      }
      loading={loading}
      dataSource={dataSource}
      columns={columns}
      pagination={{
        current: page,
        pageSize,
        total: dataSource.length,
        showTotal: (total) => `共 ${total} 条`,
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
  );
};

export default FullResourceTable;
