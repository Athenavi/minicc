# W1-W6: AI CRM（客户关系管理）

> **总预估**：4.5 周 | **前置**：无 | **产出**：替代 Salesforce 核心功能

## W1: 客户数据管理（1 周）

`backend/app/crm/models.py`：
- Contact（联系人）：name, email, phone, company, tags
- Company（公司）：name, industry, size, website, address
- Lead（线索）：source, status, score, assigned_to
- Interaction（交互）：type, content, timestamp, contact_id

`backend/app/crm/api.py` — REST API + 工具：
- `crm_contact_create` / `crm_contact_search`
- `crm_company_create` / `crm_company_search`
- `crm_lead_create` / `crm_lead_convert`（线索转客户）

## W2: 销售管道管理（1 周）

- Pipeline 阶段：leads → qualified → proposal → negotiation → closed
- 概率配置：每个阶段默认赢单概率
- AI 推荐：基于历史数据推荐下一步行动
- 移动阶段自动记录时间戳

## W3: 客户 360° 视图（0.5 周）

- 统一视图：联系人详情 + 所有交互历史 + 相关商机 + 工单
- 时间线：邮件/通话/会议/工单的统一时间线
- AI 摘要：自动生成客户画像摘要

## W4: 邮件自动化（1 周）

- 邮件模板管理
- 自动跟进（基于时间/事件触发）
- 批量发送 + A/B 测试
- 打开/点击追踪

## W5: 销售预测（0.5 周）

- 基于管道阶段的加权预测
- AI 趋势分析（季节/市场因素）
- 预测准确度追踪

## W6: 报表看板（0.5 周）

- 预置看板：管道总览 / 团队绩效 / 转化率
- 自定义报表：拖拽式（复用 V0.4 ReactFlow）
- 导出：PDF/Excel

### 验收标准
- [ ] 客户/公司/线索 CRUD 正常
- [ ] 销售管道拖拽移动
- [ ] 客户 360° 视图显示完整时间线
- [ ] 邮件自动发送
- [ ] 销售预测准确率 > 70%
- [ ] 120 测试通过
