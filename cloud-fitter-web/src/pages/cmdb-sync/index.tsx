import React, { useCallback, useEffect, useState } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { Alert, Button, Card, Form, Select, Space, Typography, message } from 'antd';
import { listSystems, SystemRow } from '@/services/systemManage';
import { syncCmdbBySystemName } from '@/services/cmdbSync';

const { Paragraph, Text } = Typography;

const CmdbSyncPage: React.FC = () => {
  const { setBreadcrumb } = useModel('layout');
  const [systems, setSystems] = useState<SystemRow[]>([]);
  const [loadingList, setLoadingList] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [form] = Form.useForm();

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

  useEffect(() => {
    setBreadcrumb({ isBack: false, title: 'CMDB 同步数据' });
  }, [setBreadcrumb]);

  useEffect(() => {
    void loadSystems();
  }, [loadSystems]);

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
      } catch (e: any) {
        const errText =
          e?.data?.error || e?.response?.data?.error || e?.message || '同步失败';
        message.error(errText);
      } finally {
        setSyncing(false);
      }
    } catch {
      // validate only
    }
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
                中是否存在与本地系统<strong>相同 system_id</strong> 的系统记录。若不存在，将提示「CMDB中没有相同系统信息」；若存在，则按日常全量任务相同规则，对<strong>该单一系统</strong>写入系统节点、K8S、主机、中间件等数据。
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
      </Space>
    </div>
  );
};

export default CmdbSyncPage;
