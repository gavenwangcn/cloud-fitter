/** 与后端 proto JSON 字段一致（grpc-gateway 默认 lowerCamelCase） */

export const PROVIDER_FILTERS = [
  { text: 'ali', value: 'ali' },
  { text: 'tencent', value: 'tencent' },
  { text: 'huawei', value: 'huawei' },
  { text: 'aws', value: 'aws' },
];

export type ResourceFieldDef = {
  dataIndex: string;
  title: string;
  /** 是否按云类型筛选（仅对 provider 列有意义） */
  filter?: boolean;
};

/** pbecs.EcsInstance */
export const ECS_FIELDS: ResourceFieldDef[] = [
  { dataIndex: 'provider', title: '云类型', filter: true },
  { dataIndex: 'envTagValue', title: '环境(标签)' },
  { dataIndex: 'nodeTagValue', title: '节点(标签)' },
  { dataIndex: 'accountName', title: '账号名称' },
  { dataIndex: 'instanceId', title: '实例 ID' },
  { dataIndex: 'instanceName', title: '实例名称' },
  { dataIndex: 'regionName', title: '地域' },
  { dataIndex: 'publicIps', title: '公网 IP' },
  { dataIndex: 'instanceType', title: '实例类型' },
  { dataIndex: 'imageId', title: '镜像 ID' },
  { dataIndex: 'imageName', title: '镜像/系统名称' },
  { dataIndex: 'osType', title: '操作系统类型' },
  { dataIndex: 'osBit', title: '系统位数' },
  { dataIndex: 'cpu', title: 'vCPU 数' },
  { dataIndex: 'memory', title: '内存(MB)' },
  { dataIndex: 'systemDiskSizeGb', title: '系统盘(GB)' },
  { dataIndex: 'dataDiskTotalGb', title: '数据盘合计(GB)' },
  { dataIndex: 'description', title: '描述' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'creationTime', title: '创建时间' },
  { dataIndex: 'expireTime', title: '过期时间' },
  { dataIndex: 'innerIps', title: '内网 IP' },
  { dataIndex: 'vpcId', title: 'VPC ID' },
  { dataIndex: 'resourceGroupId', title: '资源组 ID' },
  { dataIndex: 'chargeType', title: '计费类型' },
  {
    dataIndex: 'utilizationAudit',
    title: '利用率审计(CPU/内存 30d·180d 峰/均/谷%)',
  },
];

/** pbrds.RdsInstance（proto 中账号字段为 accoutName） */
export const RDS_FIELDS: ResourceFieldDef[] = [
  { dataIndex: 'provider', title: '云类型', filter: true },
  { dataIndex: 'envTagValue', title: '环境(标签)' },
  { dataIndex: 'nodeTagValue', title: '节点(标签)' },
  { dataIndex: 'accoutName', title: '账号名称' },
  { dataIndex: 'instanceId', title: '实例 ID' },
  { dataIndex: 'instanceName', title: '实例名称' },
  { dataIndex: 'regionName', title: '地域' },
  {
    dataIndex: 'instanceType',
    title: '部署形态 (Single/Ha/Replica，Ha 含复制模式)',
  },
  { dataIndex: 'engine', title: '数据库引擎' },
  { dataIndex: 'engineVersion', title: '引擎版本' },
  { dataIndex: 'instanceClass', title: '实例规格' },
  { dataIndex: 'cpu', title: 'vCPU 数' },
  { dataIndex: 'memoryMb', title: '内存(MB)' },
  { dataIndex: 'publicIps', title: '公网 IP' },
  { dataIndex: 'privateIps', title: '内网 IP' },
  { dataIndex: 'vpcId', title: 'VPC ID' },
  { dataIndex: 'port', title: '端口' },
  { dataIndex: 'chargeType', title: '计费类型' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'creationTime', title: '创建时间' },
  { dataIndex: 'expireTime', title: '过期时间' },
  {
    dataIndex: 'utilizationAudit',
    title: '利用率审计(CPU/内存 30d·180d 峰/均/谷%)',
  },
];

/** pbredis.RedisInstance（华为 DCS 等走 Redis 接口） */
export const DCS_FIELDS: ResourceFieldDef[] = [
  { dataIndex: 'provider', title: '云类型', filter: true },
  { dataIndex: 'envTagValue', title: '环境(标签)' },
  { dataIndex: 'nodeTagValue', title: '节点(标签)' },
  { dataIndex: 'accoutName', title: '账号名称' },
  { dataIndex: 'instanceId', title: '实例 ID' },
  { dataIndex: 'instanceName', title: '实例名称' },
  { dataIndex: 'regionName', title: '地域' },
  { dataIndex: 'specCode', title: '规格编码(spec_code)' },
  { dataIndex: 'cpu', title: 'vCPU 数' },
  { dataIndex: 'size', title: '总内存(MB)' },
  { dataIndex: 'usedMemoryMb', title: '已用内存(MB)' },
  { dataIndex: 'publicIps', title: '公网 IP' },
  { dataIndex: 'privateIps', title: '内网 IP' },
  { dataIndex: 'vpcId', title: 'VPC ID' },
  { dataIndex: 'chargeType', title: '计费类型' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'creationTime', title: '创建时间' },
  { dataIndex: 'expireTime', title: '过期时间' },
  {
    dataIndex: 'memoryUtilizationAudit',
    title: '内存利用率审计(30d·180d 峰/均/谷%)',
  },
];

/** pbkafka.KafkaInstance（DMS Kafka / CKafka 等） */
export const DMS_FIELDS: ResourceFieldDef[] = [
  { dataIndex: 'provider', title: '云类型', filter: true },
  { dataIndex: 'accoutName', title: '账号名称' },
  { dataIndex: 'instanceId', title: '实例 ID' },
  { dataIndex: 'instanceName', title: '实例名称' },
  { dataIndex: 'regionName', title: '地域' },
  { dataIndex: 'endPoint', title: '接入点' },
  { dataIndex: 'topicNumLimit', title: 'Topic 上限' },
  { dataIndex: 'distSize', title: '磁盘(GB)' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'createTime', title: '创建时间' },
  { dataIndex: 'expiredTime', title: '过期时间' },
];

/** pbcce.CceCluster（华为云 CCE 等；对应 metadata/spec/status） */
export const CCE_FIELDS: ResourceFieldDef[] = [
  { dataIndex: 'provider', title: '云类型', filter: true },
  { dataIndex: 'nodeTagValue', title: '节点(标签)' },
  { dataIndex: 'accoutName', title: '账号名称' },
  { dataIndex: 'regionName', title: '地域' },
  { dataIndex: 'clusterName', title: '集群名称 (metadata.name)' },
  { dataIndex: 'clusterUid', title: '集群 ID (metadata.uid)' },
  { dataIndex: 'flavor', title: '集群规格 (spec.flavor)' },
  { dataIndex: 'k8sVersion', title: 'K8s 版本 (spec.version)' },
  { dataIndex: 'phase', title: '状态 (status.phase)' },
  { dataIndex: 'nodeTotal', title: '节点总数' },
  { dataIndex: 'nodeNormal', title: '正常节点数' },
  { dataIndex: 'cpuTotal', title: '节点 vCPU 合计' },
  { dataIndex: 'memoryTotalMb', title: '节点内存合计(MB)' },
];
