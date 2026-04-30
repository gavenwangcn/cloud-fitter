import React, { useCallback, useEffect, useState } from 'react';
import { useModel } from '@@/plugin-model/useModel';
import { Button, Form, Input, Modal, Select, Space, Table, message } from 'antd';
import { listCloudConfigs, CloudConfigRow } from '@/services/cloudConfig';
import {
  createSystem,
  deleteSystem,
  listSystems,
  updateSystem,
  SystemRow,
} from '@/services/systemManage';

const genSystemID = (): string => `sys-${Date.now()}-${Math.floor(Math.random() * 1000000)}`;
const DEFAULT_PAGE_SIZE = 50;
const dateRule = { pattern: /^\d{4}-\d{2}-\d{2}$/, message: '请使用 YYYY-MM-DD' };

const SystemPage: React.FC = () => {
  const { setBreadcrumb } = useModel('layout');
  const [rows, setRows] = useState<SystemRow[]>([]);
  const [configs, setConfigs] = useState<CloudConfigRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form] = Form.useForm();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [total, setTotal] = useState(0);

  const loadConfigsForModal = useCallback(async () => {
    try {
      const cfgRes = await listCloudConfigs({ page: 1, pageSize: 500 });
      setConfigs(cfgRes.configs ?? []);
    } catch (e: any) {
      message.error(e?.message || '加载云账号列表失败');
    }
  }, []);

  const loadSystems = useCallback(async () => {
    setLoading(true);
    try {
      const sysRes = await listSystems({ page, pageSize });
      setRows(sysRes.systems ?? []);
      setTotal(sysRes.total ?? 0);
    } catch (e: any) {
      message.error(e?.message || '加载系统管理数据失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    setBreadcrumb({ isBack: false, title: '系统管理' });
  }, [setBreadcrumb]);

  useEffect(() => {
    loadSystems();
  }, [loadSystems]);

  const openCreate = () => {
    setModalMode('create');
    setEditingId(null);
    void loadConfigsForModal();
    form.resetFields();
    form.setFieldsValue({ systemId: genSystemID() });
    setModalOpen(true);
  };

  const openEdit = (row: SystemRow) => {
    setModalMode('edit');
    setEditingId(row.id);
    void loadConfigsForModal();
    form.setFieldsValue({
      name: row.name,
      systemId: row.systemId,
      intro: row.intro,
      onlineTime: row.onlineTime,
      status: row.status,
      accountIds: row.accountIds ?? [],
    });
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setEditingId(null);
    form.resetFields();
  };

  const submit = async () => {
    try {
      const v = await form.validateFields();
      if (modalMode === 'create') {
        await createSystem({
          name: v.name,
          intro: v.intro,
          systemId: v.systemId,
          onlineTime: v.onlineTime,
          status: v.status,
          accountIds: v.accountIds ?? [],
        });
        message.success('系统创建成功');
      } else {
        if (editingId == null) {
          return;
        }
        await updateSystem({
          id: editingId,
          systemId: v.systemId,
          intro: v.intro,
          onlineTime: v.onlineTime,
          status: v.status,
          accountIds: v.accountIds ?? [],
        });
        message.success('已保存');
      }
      closeModal();
      loadSystems();
    } catch (e: any) {
      if (e?.errorFields) {
        return;
      }
      message.error(e?.message || e?.data?.error || (modalMode === 'create' ? '创建失败' : '保存失败'));
    }
  };

  const onDelete = (row: SystemRow) => {
    Modal.confirm({
      title: '确认删除系统',
      content: `将删除系统「${row.name}」，是否继续？`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteSystem(row.id);
          message.success('系统已删除');
          loadSystems();
        } catch (e: any) {
          message.error(e?.message || e?.data?.error || '删除失败');
        }
      },
    });
  };

  return (
    <div className="pageContent">
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" onClick={openCreate}>
          新增
        </Button>
      </Space>

      <Table<SystemRow>
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
          { title: '系统名称', dataIndex: 'name', align: 'center' },
          { title: '系统功能简介', dataIndex: 'intro', align: 'center' },
          { title: '系统ID', dataIndex: 'systemId', align: 'center' },
          { title: '上线时间', dataIndex: 'onlineTime', align: 'center' },
          { title: '状态', dataIndex: 'status', align: 'center' },
          {
            title: '关联账号',
            dataIndex: 'accountNames',
            align: 'center',
            render: (names?: string[]) => (names?.length ? names.join('，') : '-'),
          },
          {
            title: '操作',
            key: 'action',
            align: 'center',
            width: 160,
            render: (_: unknown, record) => (
              <Space size={4}>
                <Button type="link" size="small" onClick={() => openEdit(record)}>
                  编辑
                </Button>
                <Button danger type="link" size="small" onClick={() => onDelete(record)}>
                  删除
                </Button>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={modalMode === 'create' ? '新增系统' : '编辑系统'}
        open={modalOpen}
        onOk={submit}
        onCancel={closeModal}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="系统名称"
            rules={modalMode === 'create' ? [{ required: true, message: '请输入系统名称' }] : []}
          >
            <Input
              placeholder="系统名称"
              autoComplete="off"
              readOnly={modalMode === 'edit'}
              disabled={modalMode === 'edit'}
            />
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
              placeholder="系统ID"
              addonAfter={
                modalMode === 'create' ? (
                  <Button
                    type="link"
                    size="small"
                    style={{ padding: 0 }}
                    onClick={() => form.setFieldsValue({ systemId: genSystemID() })}
                  >
                    生成
                  </Button>
                ) : null
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
          <Form.Item
            name="onlineTime"
            label="上线时间"
            rules={[{ required: true, message: '请选择上线时间' }, dateRule]}
            getValueFromEvent={(e) => e?.target?.value}
          >
            <Input type="date" style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item
            name="status"
            label="状态"
            rules={[{ required: true, message: '请选择状态' }]}
          >
            <Select
              placeholder="请选择状态"
              options={[
                { label: '上线', value: '上线' },
                { label: '建设中', value: '建设中' },
                { label: '下线', value: '下线' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default SystemPage;
