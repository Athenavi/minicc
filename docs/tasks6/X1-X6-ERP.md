# X1-X6: AI ERP（企业资源规划）

> **总预估**：5 周 | **前置**：Phase W

## X1: 采购管理（1 周）

`backend/app/erp/purchase.py`：
- 供应商管理（supplier_id, name, contact, rating）
- 采购订单（PO number, items, quantity, price, status）
- 入库管理（goods received, quality check）
- AI 推荐供应商（基于价格/交货期/质量评分）

## X2: 库存管理（1 周）

- 实时库存（warehouse, location, SKU, quantity）
- 库存预警（低库存自动通知）
- 自动补货建议（基于历史消耗 + 季节因素）
- 盘点管理

## X3: 订单管理（1 周）

- 销售订单（order_id, customer, items, amount, status）
- 发货管理（shipment tracking, logistics）
- 退货管理（RMA processing）
- AI 订单异常检测（异常大单/重复订单）

## X4: 财务管理（1 周）

- 发票管理（invoices, payment tracking）
- 应收/应付（aging report）
- 总账（chart of accounts, journal entries）
- AI 费用分类（自动识别费用类别）

## X5: 生产管理（0.5 周）

- BOM（Bill of Materials）
- 工单管理（production order, routing）
- 产能规划

## X6: 财务报表（0.5 周）

- 利润表 / 资产负债表 / 现金流量表
- AI 财务分析（异常波动检测）
- 导出：PDF/Excel/XML

### 验收标准
- [ ] 采购/库存/订单完整闭环
- [ ] 财务模块支持开票/收款
- [ ] 报表自动生成
- [ ] AI 异常检测准确率 > 80%
- [ ] 120 测试通过
