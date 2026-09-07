# 账单批量导出费用 — Design Spec

**日期：** 2026-09-07  
**状态：** 已确认  
**范围：** 账单汇总页批量导出多账号、多月份费用明细为单表 Excel

## 1. 背景与目标

账单汇总页（`cloud-fitter-web/src/pages/billing`）当前仅支持单账号/单系统 + 单月查询（`POST /apis/billing/by-account`）。业务需要按云账号配置列表，在起止月份范围内批量拉取大类费用明细，并下载为 Excel，便于对账与归档。

**成功标准：**

- 在「云账号（配置名称）」后可选择开始/结束年月并导出
- 导出覆盖下拉框中全部云账号配置 × 起止月（含两端）
- Excel 单表，行序为账号 → 月份 → 明细；仅含「消费合计」非空行
- 无有效数据时仍下载带表头的空文件
- 单次查询失败跳过并继续；服务端有可排查的明细日志
- 华为云调用超时错误自动重试 1 次

## 2. 非目标

- 不改造 gRPC / protobuf BillingService（走现有 JSON API 模式）
- 不改其他资源页（ECS/RDS 等）的 `CloudAccountBar` 行为
- 不做前端分页展示批量结果；不做多 Sheet
- 不引入月份跨度硬上限（长区间可接受较慢）
- 不在 Excel 中写入失败汇总行（失败仅记日志）

## 3. 架构

```
[账单页 UI]  startMonth/endMonth + 导出
      |  POST /apis/billing/batch-export  (blob)
      v
[jsonapi.BillingBatchExport]  校验月份 → store.List() 全部账号
      |  对每个 (provider, accountName, YYYY-MM)
      v
[billing.ListSummary + 华为超时重试包装]
      |  过滤 totalConsumeAmount 非空
      v
[excelize 单表] → HTTP 附件下载
```

复用现有 `billing.ListSummary`（与 `by-account` 同源），保证导出数据与图 2 表格一致。

## 4. 前端设计

### 4.1 布局

仅在账单页修改。在「云账号（配置名称）」选择器之后增加：

1. 开始时间：`DatePicker picker="month"`（与现有「账单月份」一致）
2. 结束时间：同上
3. 「导出」按钮

实现方式：给 `CloudAccountBar` 增加可选 `extra?: React.ReactNode`（或 `children`），账单页传入起止月与导出按钮；其他页面不传，行为不变。现有「账单月份」单月查询控件保留，职责不变。

### 4.2 交互

- 导出按钮启用条件：开始、结束均非空，且 `startMonth <= endMonth`（按 `YYYY-MM` 字符串或 dayjs 比较均可）
- 点击后：`loading`、禁用防重入；请求成功触发浏览器下载；失败 `message.error`
- 请求使用 `responseType: 'blob'`（或等价），从 `Content-Disposition` 取文件名，缺省 `billing-export.xlsx`

### 4.3 前端调用

新增 `cloud-fitter-web/src/pages/billing/service.ts`（或同目录工具）方法：

```ts
exportBillingBatch(startMonth: string, endMonth: string): Promise<Blob>
// POST /apis/billing/batch-export
// body: { startMonth, endMonth }
```

账号列表由后端自行枚举，前端不传账号列表。

## 5. 后端设计

### 5.1 路由

在 `main.go` 的 JSON 路由分支增加：

```
POST /apis/billing/batch-export → jsonapi.BillingBatchExport(w, r, store)
```

与 `by-account` / `by-system-id` 并列；需要 `*configstore.Store` 以 `List()` 全部云账号。

### 5.2 请求 / 响应

**Request JSON：**

```json
{
  "startMonth": "2026-01",
  "endMonth": "2026-05"
}
```

校验：

- 二者必填，格式 `YYYY-MM`（解析失败 → 400）
- `startMonth <= endMonth`，否则 → 400

**Response：**

- `200`，`Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
- `Content-Disposition: attachment; filename="billing-export-<start>-<end>.xlsx"`
- Body：xlsx 字节流
- 无有效行：仍 200 + 仅含表头的空表

### 5.3 账号与月份枚举

1. `configs, err := store.List()`（与前端 `/apis/configs` 同源，顺序建议 `ORDER BY id`）
2. 生成闭区间月份列表：`[startMonth, …, endMonth]`
3. 双重循环：**外层账号、内层月份**（保证 Excel 行序：账号 → 月份 → 明细）
4. 每次调用：

```go
billing.ListSummary(ctx, &pbbilling.ListBillingSummaryReq{
  Provider:     pbtenant.CloudProvider(cfg.Provider),
  AccountName:  cfg.Name,
  BillingCycle: month,
})
```

5. 对 `resp.Rows`：仅当 `TotalConsumeAmount` 有值（proto optional / 非 nil）时写入导出缓冲；空消费跳过（与 UI「—」一致）

### 5.4 华为云超时重试

在批量导出路径对 **华为云**（`CloudProvider_huawei`）包装：

1. 第一次调用 `ListSummary`
2. 若错误判定为超时（error 文本含 `timeout` / `Timeout` / `deadline exceeded` / `i/o timeout` 等，大小写不敏感匹配即可），则 **再调用 1 次**
3. 重试成功或失败均打日志；重试后仍失败则计入失败列表并继续下一账号/月份
4. 非华为云、非超时错误：不重试，直接记失败并继续

实现位置建议：`internal/server/billing` 内新增 `ListSummaryWithRetry`（或仅 batch-export handler 内局部包装），避免改变现有 `by-account` 单次查询语义（除非后续明确要求统一重试）。

### 5.5 失败策略

- 单次账号/月份失败：`glog.Errorf` 记录 provider、account、month、err；加入内存 `failures []string`；**不中断**整次导出
- 全部结束后：`glog.Infof` 汇总成功次数、失败次数、导出行数、失败列表
- 接口仍返回 Excel（可能为空表）

### 5.6 Excel 单表

依赖：`github.com/xuri/excelize/v2`（写入 `go.mod`）。

Sheet 名：`费用明细`（或默认 `Sheet1`）。

| 列 | 表头 | 数据来源 |
|----|------|----------|
| A | 序号 | 自增 1..N（仅写入行） |
| B | 云类型 | provider 中文（与前端 `PROVIDER_ENUM_CN` / `providerLabel` 一致） |
| C | 账号/范围 | `accountName` |
| D | 账单月份 | `billingCycle` |
| E | 资源大类 | `category` |
| F | 消费合计 | `totalConsumeAmount`（两位小数） |
| G | 币种 | `currency` |
| H | 汇总行数 | `sourceRowCount` |

### 5.7 日志规范（便于排查）

| 时机 | 级别 | 内容示例 |
|------|------|----------|
| 开始 | Info | `billing batch-export start accounts=%d start=%s end=%s months=%d queries=%d` |
| 单次成功 | Info | `billing batch-export ok account=%s provider=%d month=%s rows=%d kept=%d elapsed=%v` |
| 华为超时重试 | Warning | `billing batch-export huawei timeout, retry once account=%s month=%s err=%v` |
| 重试结果 | Info/Error | `billing batch-export huawei retry ok/fail ...` |
| 单次失败 | Error | `billing batch-export fail account=%s provider=%d month=%s err=%v` |
| 结束 | Info | `billing batch-export done success=%d fail=%d exportRows=%d failures=[...] elapsed=%v` |

## 6. 错误处理与边界

| 场景 | 行为 |
|------|------|
| 月份格式非法 / start > end | 400 JSON 错误 |
| `store.List` 失败 | 500 |
| 配置列表为空 | 仍返回带表头空 Excel |
| 全部查询失败或全部消费为空 | 带表头空 Excel + 日志含失败列表 |
| 客户端取消请求 | 尊重 `ctx.Done()`，尽快停止后续循环（已生成部分可不强制落盘；以实现简洁为准：检查 ctx 后 break，若已有缓冲可仍返回或 499——推荐检查 ctx 后中止并返回 499/500，避免半成品文件语义不清。**本规格选定：检测到取消则停止循环并返回错误，不返回半成品文件。**） |

## 7. 测试要点

- 月份展开：`2026-01`～`2026-03` → 3 个月；跨年 `2025-11`～`2026-01`
- 过滤：消费为空的行不进 Excel
- 行序：账号稳定顺序（config id）× 月份升序 × 明细顺序与 `ListSummary` 一致
- 失败跳过：mock 某一账号失败，其余仍导出
- 华为超时重试：mock 首次 timeout、二次成功 → 只重试一次且有数据
- 空结果：无配置 / 全空消费 → 仅表头文件可打开
- 前端：按钮启停条件；下载成功

## 8. 文件改动清单（预期）

**后端**

- `main.go` — 注册路由
- `internal/server/jsonapi/billing_batch_export.go` — 新 handler
- `internal/server/billing/summary.go`（或同包新文件）— 可选重试包装
- `go.mod` / `go.sum` — 增加 excelize

**前端**

- `cloud-fitter-web/src/components/CloudAccountBar/index.tsx` — 可选 `extra`
- `cloud-fitter-web/src/pages/billing/index.tsx` — 起止月 + 导出
- `cloud-fitter-web/src/pages/billing/service.ts` — 导出 API

## 9. 已确认决策

| 决策 | 选择 |
|------|------|
| 实现位置 | 后端批量接口 + excelize |
| 失败策略 | 跳过并继续，失败写日志 |
| 无数据 | 返回带表头空 Excel |
| Excel | 单表 |
| 华为超时 | 重试 1 次 |
| 账号范围 | 全部云账号配置（与下拉同源） |
| 前端超时顾虑 | 由后端聚合，前端只等一次下载 |

## 10. 开放项

无。实现阶段若 excelize API 或 proto 字段 optional 判定需微调，以与现有 `ListBillingSummaryResp` 字段为准，不改变本规格行为。
