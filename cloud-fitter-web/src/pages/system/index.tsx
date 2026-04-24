import React, { useEffect, useState } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { Button, Form, Input, Modal, Select, Space, Table, message } from 'antd';
import { listCloudConfigs, CloudConfigRow } from '@/services/cloudConfig';
import { createSystem, listSystems, SystemRow } from '@/services/systemManage';

const genSystemID = (): string => `sys-${Date.now()}-${Math.floor(Math.random() * 1000000)}`;

const SystemPage: React.FC = () => {
  const { setBreadcrumb } = useModel('layout');
  const [rows, setRows] = useState<SystemRow[]>([]);
  const [configs, setConfigs] = useState<CloudConfigRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();

  const load = async () => {
    setLoading(true);
    try {
      const [sysRes, cfgRes] = await Promise.all([listSystems(), listCloudConfigs()]);
      setRows(sysRes.systems ?? []);
      setConfigs(cfgRes.configs ?? []);
    } catch (e: any) {
      message.error(e?.message || '加载系统管理数据失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setBreadcrumb({ isBack: false, title: '系统管理' });
    load();
  }, []);

  const submit = async () => {
    try {
      const v = await form.validateFields();
      await createSystem({
        name: v.name,
        intro: v.intro,
        systemId: v.systemId,
        accountIds: v.accountIds ?? [],
      });
      message.success('系统创建成功');
      setModalOpen(false);
      form.resetFields();
      load();
    } catch (e: any) {
      if (e?.errorFields) {
        return;
      }
      message.error(e?.message || e?.data?.error || '创建失败');
    }
  };

  return (
    <div className="pageContent">
      <Space style={{ marginBottom: 16 }}>
        <Button
          type="primary"
          onClick={() => {
            setModalOpen(true);
            form.setFieldsValue({ systemId: genSystemID() });
          }}
        >
          新增
        </Button>
      </Space>

      <Table<SystemRow>
        rowKey="id"
        loading={loading}
        dataSource={rows}
        pagination={false}
        columns={[
          { title: '系统名称', dataIndex: 'name', align: 'center' },
          { title: '系统功能简介', dataIndex: 'intro', align: 'center' },
          { title: '系统ID', dataIndex: 'systemId', align: 'center' },
          {
            title: '关联账号',
            dataIndex: 'accountNames',
            align: 'center',
            render: (names?: string[]) => (names?.length ? names.join('，') : '-'),
          },
        ]}
      />

      <Modal
        title="新增系统"
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
            name="name"
            label="系统名称"
            rules={[{ required: true, message: '请输入系统名称' }]}
          >
            <Input placeholder="请输入系统名称" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="intro"
            label="系统功能简介"
            rules={[{ required: true, message: '请输入系统功能简介' }]}
          >
            <Input.TextArea rows={4} placeholder="请输入系统功能简介" />
          </Form.Item>
          <Form.Item
            name="systemId"
            label="系统ID"
            rules={[{ required: true, message: '请输入系统ID' }]}
          >
            <Input
              placeholder="请输入系统ID"
              addonAfter={
                <Button
                  type="link"
                  size="small"
                  style={{ padding: 0 }}
                  onClick={() => form.setFieldsValue({ systemId: genSystemID() })}
                >
                  生成
                </Button>
              }
            />
          </Form.Item>
          <Form.Item
            name="accountIds"
            label="关联账号"
            rules={[{ required: true, message: '请选择关联账号' }]}
          >
            <Select
              mode="multiple"
              placeholder="请选择配置管理中的账号"
              options={configs.map((c) => ({ label: c.name, value: c.id }))}
              allowClear
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default SystemPage;
