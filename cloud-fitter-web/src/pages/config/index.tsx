import React, { useEffect, useState, useCallback } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { Button, Form, Input, Modal, Popconfirm, Select, Space, Table, message } from 'antd';
import {
  createCloudConfig,
  deleteCloudConfig,
  listCloudConfigs,
  providerLabel,
  CloudConfigRow,
} from '@/services/cloudConfig';

const PROVIDER_OPTIONS = [
  { label: '0 阿里云', value: 0 },
  { label: '1 腾讯云', value: 1 },
  { label: '2 华为云', value: 2 },
];

const DEFAULT_PAGE_SIZE = 50;

const ConfigPage: React.FC = () => {
  const { setBreadcrumb } = useModel('layout');
  const [rows, setRows] = useState<CloudConfigRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [total, setTotal] = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listCloudConfigs({ page, pageSize });
      setRows(res.configs ?? []);
      setTotal(res.total ?? 0);
    } catch (e: any) {
      message.error(e?.message || '加载配置失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    setBreadcrumb({ isBack: false, title: '配置管理' });
  }, [setBreadcrumb]);

  useEffect(() => {
    load();
  }, [load]);

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

  const onDelete = async (id: number) => {
    try {
      await deleteCloudConfig(id);
      message.success('已删除');
      if (rows.length <= 1 && page > 1) {
        setPage(page - 1);
      } else {
        load();
      }
    } catch (e: any) {
      message.error(e?.message || e?.data?.error || '删除失败');
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
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          pageSizeOptions: ['20', '50', '100', '200'],
          defaultPageSize: DEFAULT_PAGE_SIZE,
        }}
        onChange={(pg) => {
          setPage(pg.current ?? 1);
          setPageSize(pg.pageSize ?? DEFAULT_PAGE_SIZE);
        }}
        columns={[
          { title: 'ID', dataIndex: 'id', align: 'center' },
          {
            title: '提供商',
            dataIndex: 'provider',
            align: 'center',
            render: (p: number) => providerLabel(p),
          },
          { title: '名称', dataIndex: 'name', align: 'center' },
          {
            title: '操作',
            key: 'actions',
            align: 'center',
            width: 100,
            render: (_: unknown, record) => (
              <Popconfirm
                title="确定删除该云账号？"
                description="若系统管理中仍有系统引用此账号，将无法删除。"
                onConfirm={() => onDelete(record.id)}
                okText="删除"
                cancelText="取消"
              >
                <Button type="link" danger size="small">
                  删除
                </Button>
              </Popconfirm>
            ),
          },
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
