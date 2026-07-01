# 封闭区域 Revit 2026 插件

## Goal
开发 Revit 2026 插件，从详图线集合中检测封闭区域，生成最大/最小填充区域。

## Commands
- **MaxRegionCommand**: 选择白色曲线 → 生成1个最大封闭区域（黄色填充区域）
- **MinRegionCommand**: 选择白色曲线 → 生成N个最小封闭区域（洋红色填充区域）

## Core Algorithm
1. 曲线求交与分割
2. 平面图构建（图数据结构）
3. 最小环检测（最小封闭区域）
4. 区域合并（最大封闭区域）
5. 生成 FilledRegion

## Constraints
- 不使用 Revit 原生 Room 功能
- 支持直线/圆弧/样条曲线
- 处理相交/重叠/T形/十字连接/孤岛
- 浮点误差容差（<5单位）
- 自动删除上次生成结果

## Success Criteria
- [ ] 项目可编译
- [ ] MaxRegionCommand 找到最大封闭区域并生成黄色 FillRegion
- [ ] MinRegionCommand 找到所有最小封闭区域并生成洋红色 FillRegion
- [ ] 正确处理曲线相交与分割
- [ ] 正确处理重叠与去重
- [ ] 支持 T 形/十字连接
- [ ] 支持孤岛区域
- [ ] 再次执行时自动清空上次结果
