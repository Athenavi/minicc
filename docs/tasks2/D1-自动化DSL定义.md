# D1：自动化 DSL 定义

> **所属阶段**：Phase D - 工作流引擎
> **预估工时**：3 天
> **依赖**：Phase A-C 的核心工具

---

## 1. 任务目标

设计并实现工作流自动化 DSL（领域特定语言），用 YAML/JSON 定义多步骤自动化流程，支持条件、循环、变量、错误处理等核心工作流语义。

对标影刀的工作流编排，但 AI 原生优势：AI 自动从自然语言生成 DSL。

## 2. 详细子任务

### 2.1 DSL Schema 设计

- [ ] `automator/dsl.py`

```python
class WorkflowStep(BaseModel):
    id: str = Field(description="步骤唯一标识")
    tool: str = Field(description="工具名，如 browser.click")
    params: dict[str, Any] = Field(default_factory=dict, description="工具参数")
    if_: Optional["WorkflowCondition"] = Field(default=None, alias="if")
    loop: Optional["WorkflowLoop"] = None
    timeout: int = Field(default=60, ge=1)
    retry: int = Field(default=0, ge=0, le=5)
    on_error: Optional["WorkflowErrorHandler"] = None

class WorkflowCondition(BaseModel):
    condition: str = Field(description="条件表达式，如 '{{ .steps.prev.success }}'")
    then: list[WorkflowStep] = Field(description="条件满足时执行")
    else_: Optional[list[WorkflowStep]] = Field(default=None, alias="else")

class WorkflowLoop(BaseModel):
    over: str = Field(description="遍历对象，如 '{{ .data.items }}'")
    as_: str = Field(description="循环变量名")
    steps: list[WorkflowStep]

class WorkflowTrigger(BaseModel):
    type: Literal["cron", "webhook", "manual", "event"]
    cron: str | None = None  # cron 表达式
    event: str | None = None  # 事件类型

class WorkflowDefinition(BaseModel):
    name: str
    description: str | None = None
    version: str = "1.0"
    trigger: WorkflowTrigger | None = None
    variables: dict[str, Any] = Field(default_factory=dict)
    steps: list[WorkflowStep]
```

### 2.2 DSL 示例

```yaml
name: "日报表下载"
description: "每天自动下载销售报表"
version: "1.0"
trigger:
  type: cron
  cron: "0 9 * * 1-5"  # 工作日9点
variables:
  report_date: "{{ .date.YYYYMMDD }}"
  save_dir: "/reports"
steps:
  - id: login
    tool: browser.navigate
    params:
      url: "{{ .env.ERP_URL }}"
    timeout: 30
    retry: 2
    on_error:
      action: notify
      message: "ERP 登录失败"

  - id: fill_username
    tool: browser.fill
    params:
      selector: "#username"
      value: "{{ .env.ERP_USER }}"

  - id: fill_password
    tool: browser.fill
    params:
      selector: "#password"
      value: "{{ .env.ERP_PASS }}"

  - id: click_login
    tool: browser.click
    params:
      selector: "#login-btn"
    timeout: 10

  - id: wait_report
    tool: browser.wait
    params:
      selector: "#report-table"
      timeout: 15

  - id: extract_data
    tool: web.extract_table
    params:
      selector: "#report-table"
      max_rows: 5000

  - id: save_excel
    tool: excel.write
    params:
      path: "{{ .save_dir }}/sales_{{ .report_date }}.xlsx"
      values: "{{ .steps.extract_data.result }}"

  - id: send_email
    tool: email.send
    params:
      to: "manager@company.com"
      subject: "日报表 - {{ .report_date }}"
      body: "报表已生成，请查看附件。"
      attachments:
        - "{{ .save_dir }}/sales_{{ .report_date }}.xlsx"
```

### 2.3 变量系统

- [ ] 内置变量：环境变量、日期、步骤结果
- [ ] 用户变量：DSL 中定义的 variables
- [ ] 引用语法：`{{ .steps.step_id.result }}`, `{{ .env.VAR_NAME }}`
- [ ] 表达式函数：`{{ .date.YYYYMMDD }}`, `{{ .string.lower }}`

### 2.4 DSL 验证器

- [ ] Schema 校验（Pydantic）
- [ ] 步骤引用检查（确保 step_id 存在）
- [ ] 变量引用检查
- [ ] 循环终止条件检查

### 2.5 测试

- [ ] 完整 DSL 解析
- [ ] 变量替换
- [ ] 条件表达式计算
- [ ] 循环展开
- [ ] 错误处理配置
- [ ] Schema 验证失败场景

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-------|:---------|
| 1 | DSL 可正确定义多步骤工作流 | 解析并执行 |
| 2 | 支持条件分支 | if/else 正确执行 |
| 3 | 支持循环 | 循环遍历列表项 |
| 4 | 变量引用正确替换 | 步骤参数含正确值 |
| 5 | 错误处理机制生效 | 超时/失败后执行 on_error |
| 6 | 触发器配置正确 | cron 表达式解析 |

## 4. 参考资源

- [Prefect 工作流引擎](https://docs.prefect.io/)
- [GitHub Actions Workflow 语法](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions)
