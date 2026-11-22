# 内容过滤词表：第三方来源与改动说明

本文是 `content_filter` 段（第 11 个运行时配置段，见 [ADR 索引](../adr/README.md) 中的 ADR-0032
「全局内容过滤」）初始词表的**来源声明与 NOTICE**。词表数据本身**不随仓库分发**
（见下文「数据不落仓库」），但来源、许可证与我们对词表做过的改动必须随产品一起存在。

## 来源

| 项 | 内容 |
|:--|:--|
| 上游项目 | [`houbb/sensitive-word-data`](https://github.com/houbb/sensitive-word-data)（jdk 敏感词库，配套 `houbb/sensitive-word`） |
| 使用的文件 | `src/main/resources/sensitive_word_dict.txt`（黑名单）、`src/main/resources/sensitive_word_tags.txt`（带标签的黑名单） |
| 上游分支 / 提交 | `main` / `fe6fc2921836217b8c90619db81b24af8b22d80f`（2025-07-19） |
| 许可证 | **Apache License 2.0**：全文副本见仓库内 [`apache-2.0.txt`](./apache-2.0.txt)（即上游 `LICENSE.txt` 原文），或 <http://www.apache.org/licenses/LICENSE-2.0> |
| 上游再上游署名 | 上游 `LICENSE.txt` 附录保留了 **iBase4J** 的版权声明：`版权所有 2016 ShenHuaJie iBase4J@163.com`，同样以 Apache-2.0 授权。iBase4J 项目见 <https://github.com/iBase4J/iBase4J> |

上游词库的标签只有 5 类（数字表示）：`0 政治 / 1 毒品 / 2 色情 / 3 赌博 / 4 违法`。

## 本仓库对词表做过的改动

词表**不是上游原样分发**。以下改动由本仓库完成，属于 Apache-2.0 §4(b) 要求的
"被修改文件需说明"：

1. **类目重映射 5 → 7**：上游 `0/1/2/3` 直接对应本站 `politics/drugs/porn/gambling`；
   上游 `4 违法` 按关键词表再细分为 `fake_docs`（发票、证件、印章、假币…）、
   `violence`（枪支、弹药、刀械、爆炸物…）、`drugs`（冰毒、大麻、海洛因…），
   都不命中时兜底 `fraud`（诈骗、刷单、代开发票以外的推广话术）。
   **`violence` 的细分词表不在仓库里**：它是暴恐类词，按 ADR-0032 决策 8 只允许存在
   于部署侧，构建时由 `BLOGX_VIOLENCE_HINTS` 指向的 gitignored 文件注入
   （`scripts/content-filter/lib/violenceHints.mjs`）；没注入时这类词兜底归 `fraud`
   —— 类目会不准，但词本身仍进库、仍拦截。
2. **多标签重排**：一个词带上游多个标签时按 `政治 > 毒品 > 违法 > 赌博 > 色情`
   取第一个，目的是让 block 档类目优先于 watch 档类目。
3. **档位**：`politics / drugs / violence / fake_docs / fraud → block`（拦截词），
   `porn / gambling → watch`（观察词）。两档在过滤层行为相同（都替换为等长 `*`），
   差别只在后台风险页的标识颜色与处置口径。
4. **过滤与截断**：CJK 少于 2 个字符的词丢弃（单字词与纯拉丁/数字词误报压过收益）、
   超过 24 个字符的拼接长噪音词丢弃、按小写去重、按类目比例截断到 30000 条。
   规则实现在 [`scripts/content-filter/lib/wordlist.mjs`](../../scripts/content-filter/lib/wordlist.mjs)，
   有 `node --test` 覆盖。
5. **默认排除 `politics` 与 `violence`**：这两类的词条只允许在**部署侧**直接导入数据库，
   不进仓库、不进默认 `settings.yaml`、不进 CI 与日志（见 ADR-0032 的安全约束）。
   默认输出**不含**这两类；只有显式 `--include-politics` / `--include-violence` /
   `--include-all` 才会生成。注意这不只是「输出时排除」：连**用来判类目的触发词**
   也不在仓库里（`violence` 那一组原本写在 `lib/wordlist.mjs`，已移出，见第 1 条）。
6. **未标注词默认不用**：上游黑名单里没有标签的 2 万余条词没有可信类目，默认丢弃；
   `--include-untagged` 会把它们按"违法 → 兜底诈骗"纳入。

除以上处理外，本仓库未对词条内容做增删改写；条目本身仍受上游许可证约束。

## 数据不落仓库

词表是敏感数据：它一旦进入 git 历史，就等于永久公开（`git log` 无法真正遗忘）。
因此：

- 构建产物默认写 `.gotmp/content-filter-words.tsv`，`.gotmp/` 已在 `.gitignore` 中忽略；
- 从上游下载的文件落系统临时目录，不落工作区；
- 导入命令（`backend -filter-import <file>`）只读本地文件，日志只记数量与类目分布，
  **不记词条**；
- 任何情况下不要把生成的 TSV / NDJSON 贴进 issue、PR、聊天或 CI 日志。

## 复现步骤

```bash
# 1) 生成词表（首次无本地副本时会从 raw.githubusercontent.com 下载到临时目录）
node scripts/content-filter/build-wordlist.mjs --limit 30000

# 没有外网时用本地副本（仓库目录或 maven 资源目录都认）
node scripts/content-filter/build-wordlist.mjs --source ~/data/sensitive-word-data

# 2) 预演：只报告，不写库
backend -f config/settings.yaml -filter-import .gotmp/content-filter-words.tsv

# 3) 真正写入（幂等：重复执行不会产生重复词条，同词保留库里已有的档位与类目）
backend -f config/settings.yaml -filter-import .gotmp/content-filter-words.tsv -filter-import-apply
```

部署侧需要 `politics` / `violence` 词条时，在**部署机**上另行生成一份
（`--include-politics --include-violence`），只导入数据库，不提交、不外传。
生成前先把暴恐触发词写进部署侧的 `violence-hints.txt`（每行一词，gitignored）：

```bash
BLOGX_VIOLENCE_HINTS=/srv/blogx/private/violence-hints.txt \
  node scripts/content-filter/build-wordlist.mjs --include-all --out /tmp/words.tsv
```

没设 `BLOGX_VIOLENCE_HINTS` 或文件不存在时脚本**不报错**：暴恐词兜底归 `fraud`
（词仍进库、仍拦截，只是风险页里类目显示为诈骗）。日志只打印来源路径与条数，
从不打印任何词。

## 分发时的义务

如果分发物中包含词表数据（例如交付一个已导入词表的数据库快照或镜像），
必须同时带上：

1. 本文件（来源、改动说明、署名）；
2. [`apache-2.0.txt`](./apache-2.0.txt)（Apache-2.0 全文）；
3. 上游与 iBase4J 的版权声明（见上表）。

## 已知局限

- 上游词库包含少量**过宽的两字词**（如「冰糖」「农药」「卖房」一类的通用词），
  在正常文章里有误报可能。这类词归在 block 档，上线后应通过白名单（`content_filter.whitelist`）
  按站点内容逐步收敛；本仓库不再手工增删上游条目。
- 词库不含拼音、谐音、形近字变体（由 AI 层兜底，见 #393 的 Out of Scope）。
- 上游词库会持续更新；升级时重新跑构建脚本即可，改动说明的第 1–6 条不需要变。

## 主干补全（covering-stems.mjs）

上游词库里大量条目是「主干 + 渠道/交易后缀」的组合（`冰毒`、`冰毒qq`、`qq出售冰毒`）。
引擎是子串匹配，所以**主干在表里**就够；但如果某条主干只以变体形式存在，就留下覆盖漏洞
（单独写 `仿真气枪` 不命中，写 `仿真气枪出售` 反而命中）。

`scripts/content-filter/covering-stems.mjs` 从一份词表推导这类"缺失主干"，输出可直接
`-filter-import` 的 TSV。规则：含中文、2–8 个字、自身不带渠道词；两字全中文有一条变体即可，
其余要求两处证据；前缀与后缀方向都会剥。

只对 **`drugs` / `violence` / `fake_docs`** 使用它的输出。实测（2026-11-22）：`fraud`
主干（813 条）导入后，存量清洗预演从 184 处涨到 1665 处（`fraud` 1485 处），属于大面积误伤；
换成三个高信号类目后命中数一处没涨。这条经验已写进 README 的 12.2.4。
