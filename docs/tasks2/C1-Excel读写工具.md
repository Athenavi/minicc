# C1：Excel 读写工具

> **所属阶段**：Phase C - Office 自动化
> **预估工时**：4 天
> **依赖**：V0.1 工具链

---

## 1. 任务目标

实现 Excel 文件的完整读写能力，让 AI 能通过自然语言指令完成：创建/打开 Excel 文件、读写单元格、应用公式、创建图表、生成透视表等。对标影刀的"Excel 处理"能力。

## 2. 详细子任务

### 2.1 核心工具

- [ ] `tools/office/excel.py`

#### `ExcelOpenTool` — 打开/创建 Excel

```python
class ExcelOpenInput(BaseModel):
    path: str = Field(description="文件路径（不存在时创建新文件）")
    sheet_name: str | None = Field(default=None, description="工作表名称")
```

#### `ExcelReadTool` — 读取单元格/区域

```python
class ExcelReadInput(BaseModel):
    path: str
    range: str = Field(description="单元格范围，如 'A1:C10' 或 'A1'")
    sheet_name: str | None = None
    include_formulas: bool = False
```

#### `ExcelWriteTool` — 写入单元格

```python
class ExcelWriteInput(BaseModel):
    path: str
    range: str = Field(description="起始单元格，如 'A1'")
    values: list[list] = Field(description="要写入的值（二维数组）")
    sheet_name: str | None = None
```

#### `ExcelFormulaTool` — 应用公式

```python
class ExcelFormulaInput(BaseModel):
    path: str
    cell: str = Field(description="目标单元格，如 'D2'")
    formula: str = Field(description="Excel 公式，如 '=SUM(B2:B10)'")
    sheet_name: str | None = None
```

#### `ExcelChartTool` — 创建图表

```python
class ExcelChartInput(BaseModel):
    path: str
    chart_type: Literal["bar", "line", "pie", "scatter"]
    data_range: str
    title: str | None = None
    sheet_name: str | None = None
```

### 2.2 高级功能

- [ ] 样式设置（字体、颜色、边框、对齐）
- [ ] 单元格合并
- [ ] 数据筛选与排序
- [ ] 透视表生成（pivot table）
- [ ] 多工作表管理（新建/重命名/删除）
- [ ] 大文件流式读写（分块处理）

### 2.3 权限

- `ExcelReadTool` → `READ`
- `ExcelWriteTool` → `WRITE`
- `ExcelFormulaTool` → `WRITE`
- `ExcelChartTool` → `WRITE`

### 2.4 测试

- [ ] 创建新 Excel 文件
- [ ] 读写单元格数据
- [ ] 应用公式
- [ ] 创建图表
- [ ] 读取已有文件
- [ ] 大文件（10万行）读写

## 3. 验收标准

| # | 检查项 | 验证方式 |
|:-|:-------|:---------|
| 1 | AI 可创建 Excel 并写入数据 | 验证文件内容 |
| 2 | AI 可读取 Excel 数据并分析 | 验证输出正确 |
| 3 | AI 可在 Excel 中应用公式 | 验证公式生效 |
| 4 | AI 可创建图表 | 验证文件含图表对象 |
| 5 | 大文件处理不 OOM | 10万行测试通过 |

## 4. 参考资源

- [openpyxl 文档](https://openpyxl.readthedocs.io/)
