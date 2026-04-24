import React, { useEffect, useState } from 'react';
import { Select, Space } from 'antd';
import {
  CloudConfigRow,
  listCloudConfigs,
  providerLabel,
} from '@/services/cloudConfig';
import { listSystems, SystemRow } from '@/services/systemManage';

export interface CloudAccountBarProps {
  onQuery: (provider: number, accountName: string) => void;
  onQueryBySystem?: (systemName: string) => void;
  /** 清空下拉选择时回调 */
  onClear?: () => void;
}

const CloudAccountBar: React.FC<CloudAccountBarProps> = ({
  onQuery,
  onQueryBySystem,
  onClear,
}) => {
  const [configs, setConfigs] = useState<CloudConfigRow[]>([]);
  const [systems, setSystems] = useState<SystemRow[]>([]);

  useEffect(() => {
    listCloudConfigs()
      .then((r) => setConfigs(r.configs ?? []))
      .catch(() => setConfigs([]));
    listSystems()
      .then((r) => setSystems(r.systems ?? []))
      .catch(() => setSystems([]));
  }, []);

  return (
    <Space style={{ marginBottom: 16 }} align="center">
      <span>系统名称：</span>
      <Select<string>
        style={{ minWidth: 240 }}
        placeholder="按系统筛选资源"
        allowClear
        options={systems.map((s) => ({
          label: s.name,
          value: s.name,
        }))}
        onChange={(name) => {
          if (!name) {
            onClear?.();
            return;
          }
          onQueryBySystem?.(name);
        }}
      />
      <span>云账号（配置名称）：</span>
      <Select<number>
        style={{ minWidth: 300 }}
        placeholder="请选择后加载资源"
        allowClear
        options={configs.map((c) => ({
          label: `${c.name}（${providerLabel(c.provider)}）`,
          value: c.id,
        }))}
        onChange={(id) => {
          if (id == null) {
            onClear?.();
            return;
          }
          const c = configs.find((x) => x.id === id);
          if (c) {
            onQuery(c.provider, c.name);
          }
        }}
      />
    </Space>
  );
};

export default CloudAccountBar;
