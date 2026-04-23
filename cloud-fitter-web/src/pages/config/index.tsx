import React, { useEffect, useState } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { Button, Form, Input, Modal, Select, Space, Table, message } from 'antd';
import {
  createCloudConfig,
  listCloudConfigs,
  providerLabel,
  CloudConfigRow,
} from '@/services/cloudConfig';

const PROVIDER_OPTIONS = [
  { label: '0 阿里云', value: 0 },
  { label: '1 腾讯云', value: 1 },
  { label: '2 华为云', value: 2 },
];

const ConfigPage: React.FC = () => {
  const { setBreadcrumb } = useModel('layout');
  const [rows, setRows] = useState<CloudConfigRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();

  const load = async () => {
    setLoading(true);
    try {
      const res = await listCloudConfigs();
      setRows(res.configs ?? []);
    } catch (e: any) {
      message.error(e?.message || '加载配置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setBreadcrumb({ isBack: false, title: '配置管理' });
    load();
  }, []);

  const submit = async () => {
    try {
      const v = await form.validateFields();
      await createCloudConfig({
        provider: v.provider,
        name: v.name,
        accessId: v.accessId,
        accessSecret: v.accessSecret,
      });
      message.success('已保存');
      setModalOpen(false);
      form.resetFields();
      load();
    } catch (e: any) {
      if (e?.errorFields) {
        return;
      }
      message.error(e?.message || e?.data?.error || '保存失败');
    }
  };

  return (
    <div className="pageContent">
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" onClick={() => setModalOpen(true)}>
          新增配置
        </Button>
      </Space>
      <Table<CloudConfigRow>
        rowKey="id"
        loading={loading}
        dataSource={rows}
        pagination={false}
        columns={[
          { title: 'ID', dataIndex: 'id', align: 'center' },
          {
            title: '提供商',
            dataIndex: 'provider',
            align: 'center',
            render: (p: number) => providerLabel(p),
          },
          { title: '名称', dataIndex: 'name', align: 'center' },
        ]}
      />
      <Modal
        title="新增云账号配置"
        open={modalOpen}
        onOk={submit}
        onCancel={() => {
          setModalOpen(false);
          form.resetFields();
        }}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="provider"
            label="提供商"
            rules={[{ required: true, message: '请选择提供商' }]}
          >
            <Select options={PROVIDER_OPTIONS} placeholder="请选择" />
          </Form.Item>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input placeholder="账户显示名，需唯一" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="accessId"
            label="AccessKey Id (AK)"
            rules={[{ required: true, message: '请输入 AccessKey Id' }]}
          >
            <Input placeholder="accessId" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="accessSecret"
            label="AccessKey Secret (SK)"
            rules={[{ required: true, message: '请输入 AccessKey Secret' }]}
          >
            <Input.Password placeholder="accessSecret" autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default ConfigPage;
