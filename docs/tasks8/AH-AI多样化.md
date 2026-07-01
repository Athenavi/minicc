# AH1-AH5: AI 多样化 — 个体多样性

> **预估**：4.5 周 | **产出**：多 LLM 集成 + 专长分化 + 性格系统 + 身份体系 + 生命周期

## AH1: 模型多样化（1 周）
`backend/app/civilization/models.py`：
- 集成多种 LLM（Claude/GPT/DeepSeek/Llama）
- 每个 AI 个体可选择不同模型
- 模型路由（按任务类型自动选择最佳模型）
- 实现：`AICitizenCreateTool` 的 `model` 参数

## AH2: 专长分化（1 周）
- 每个 AI 有独特专业领域
- 专长类型：coder/designer/analyst/creative/coordinator
- 专长影响工具可用性和决策风格
- 实现：`AICitizenCreateTool` 的 `specialty` 参数

## AH3: 性格系统（1 周）
- 性格维度：保守/平衡/创新
- 影响决策倾向和回复风格
- 性格可随经验演化

## AH4: 身份体系（1 周）
- 唯一 ID + 名称 + 声誉分
- 交互历史档案
- 成就/贡献记录

## AH5: 生命周期（0.5 周）
- 创建（出生）→ 活跃 → 归档（退休）
- 状态自动管理
- 实现：`ai_citizen_list` 的 `status` 字段

### 代码位置
- `backend/app/civilization/__init__.py`
- 9 工具已实现：citizen_create, citizen_list, dao_propose, dao_vote, dao_list, economy_mint, economy_transfer, culture_art, diplomacy_treaty
