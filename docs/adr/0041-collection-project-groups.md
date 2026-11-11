# ADR-0041：合集 / 项目分组（4 张新表）+ 分组树成为唯一顺序真相 + 原子 compose

- 状态：已交付
- 日期：2026-11-11
- 来源：后台合集 / 项目页"一张卡片塞满全部信息、无法按主题归拢"的实际使用反馈 · 修订 [ADR-0023](./0023-outline-tree-for-collections-and-projects.md) 的"唯一排序入口"约束 · 沿用零 DDL 口径

## 背景与问题

合集与项目在后台都是**一列平铺的卡片**：标题、描述、展示开关、目录树按钮全挤在一张卡上。
运营要管十几个合集时，看不出"哪几个是同一批主题"，也没法把一批暂时不用的挪到一边。

三条结构性原因：

1. **没有分组这一层**。`collection_models.sort_order` 只能表达"谁在前谁在后"，表达不了"这几
   个属于同一批"。运营只能靠标题前缀（`[2024] xxx`）自己造分组，于是排序与语义混在一个字段里。
2. **顺序有两个真相**。列表顺序在 `collection_models.sort_order`，目录树里节点顺序在
   `collection_outline_node_models.sort_order`。前者是"合集之间"，后者是"合集内部"，
   但后台只有后者能拖 —— 想调合集之间的顺序就得改数字。
3. **保存不是原子的**。编辑弹窗里改标题是一次请求、拖完树点「保存目录树」是另一次。
   任何一次失败都会留下"标题改了但树没改"的半截状态，而用户只点了一次保存。

## 决策

1. **四张新表**，两个实体各一对（零 DDL：只新建，不改任何既有列）：

   | 表 | 作用 |
   | --- | --- |
   | `collection_group_models` | 合集分组：`title` + `sort_order` |
   | `collection_group_item_models` | 合集归属：`group_id` + `collection_id` + `sort_order` |
   | `project_group_models` | 项目分组：同构 |
   | `project_group_item_models` | 项目归属：同构 |

   - `group_id = 0` 表示**未分组**。未分组不是一种分组，它是"还没被放进任何分组"，
     不建一条永远存在的特殊行来代表它。
   - 归属表的唯一键落在 `collection_id` / `project_id` 上（**不是** `(group_id, x_id)`）：
     否则同一个合集能同时进两个分组，界面上会出现两份。
   - 归属表对实体主键有真实外键（`fk_collection_group_item_collection` 等），
     删实体时数据库会拦住悬挂行。

2. **分组树是分组的唯一顺序真相**。分组之间、组内条目、未分组段，三者顺序全部由归属表的
   `sort_order` 表达；`collection_models.sort_order` / `project_models.sort_order`
   不再是"合集之间的顺序"，只作为**未分组条目**的兜底排序（它们既没有归属行、
   也没被拖过）。列表读面因此只有一条规则：先按分组顺序、再按组内顺序、
   未分组排在最后并按 `sort_order` 兜底。这是对
   [ADR-0023](./0023-outline-tree-for-collections-and-projects.md) 约束 7「排序只经树编辑器一个入口」的修订：
   那条约束说的是**目录树内部**的层次，本 ADR 补的是**实体之间**的编排，
   两者是不同的维度，不构成第二套真相。

3. **分组 CRUD 立即生效**。建 / 改名 / 删各是一次独立提交，不参与拖拽的 10 秒防抖。
   用户点了"删除分组"却要等 10 秒才生效是不可接受的。

4. **整树换序是幂等整份覆盖**。`PUT /<kind>/groups/order` 提交的一定是界面上最终看到的
   排列；后端在集合与当前库不一致时**整单拒绝**（有人并发建 / 删了分组或条目），
   前端据此刷新后重排。宽松的"缺几个也照写"会把那条静默排丢。

5. **删分组只删编排**。分组软删 + 其条目 `group_id` 置 0，合集 / 项目本身一行不动。
   只软删分组而不动条目会让那些条目指向一个不存在的分组，读面既不在任何分组里、
   也不在未分组里 —— 界面上直接消失。

6. **一个原子 compose 端点**：`PUT /<kind>/{id}/compose` 在**一个事务**里写字段
   （标题 / 描述 / 封面）与整棵目录树。编辑弹窗里"改标题 + 拖完树"是一次保存动作；
   分成两个请求，任何一个失败都会留下半截状态，而用户没有任何线索知道该重试哪一个。

7. **分组语义只写一份**。两个实体各有一对表，但领域规则、用例、handler 都只有一份：
   表名通过 `groupTables` / `grouping.Repository` 注入。规则写两份的话，
   "未分组固定第一""删分组不删条目"这些口径早晚会漂成两种。

## 与既有决策的关系

- **不违反零 DDL**：四张表都是 `CREATE TABLE IF NOT EXISTS`，没有任何 `ALTER` / `DROP`。
- **不取代 ADR-0023 的树**：目录树仍然是"合集 / 项目内部"的层次与收录唯一真相；
  本 ADR 加的是"实体之间"的编排层，两层各自单一真相。
- **与标签 / 项目 / 热门置顶的全量换序同一条语义**（`order_mismatch` 整单拒绝）：
  四处都是"后台拿的是上一次读到的列表"，同一种失效形态用同一种处理。

## 相关代码与文档

- 领域：[`grouping`](../../backend/internal/article/domain/grouping/grouping.go)（纯规则 + 仓储契约）
- 适配器：[`group_repo.go`](../../backend/internal/article/adapter/mysql/group_repo.go)（一对表名 → 一份实现）
- 用例：[`grouping.go`](../../backend/internal/article/app/grouping.go)（CRUD / 换序 / compose）
- 端点：[`grouping_handler.go`](../../backend/internal/article/adapter/http/grouping_handler.go)
- 概念：[GLOSSARY · 合集](../../GLOSSARY.md) · [GLOSSARY · 项目](../../GLOSSARY.md)
