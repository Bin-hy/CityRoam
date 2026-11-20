# Agent Skills

技能是研究阶段的 Markdown 方法说明。系统只常驻技能目录（名称、版本、用途），正文由 `load_skill` 工具按需加载，避免把所有说明塞进每次提示词。

## 目录与文件

```
skills/
  skills.lock.json
  beijing-research-playbook/
    1.0.0/
      SKILL.md
    1.0.1/
      SKILL.md
```

`SKILL.md` 必须以 YAML 前置元数据开头：

```markdown
---
name: beijing-research-playbook
description: 用于北京行程研究：先核对闭馆日与通勤半径，再用攻略补玩法，最后检查预算与雨天替代。
version: 1.0.1
---

正文……
```

约束：

- `name` 与目录名一致，使用小写字母、数字和单个连字符。
- `version` 与版本目录名一致，必须是 `MAJOR.MINOR.PATCH` 形式的 semver。
- `description` 必填，单行，最多 300 字符；它同时是模型判断是否加载的唯一依据。
- 正文必填，最多 32 KiB；技能总数最多 32 个；所有正文合计不超过 256 KiB。
- 研究提示词里的目录行合计不超过 8 KiB；超出时启动失败而不是截断。

## 版本锁定

`skills.lock.json` 是唯一的活动版本清单：

```json
{
  "version": 1,
  "skills": {
    "beijing-research-playbook": {
      "version": "1.0.1",
      "sha256": "…"
    }
  }
}
```

- 目录里可以保留多个版本，只有锁中的版本参与加载；只要还存在可恢复的 run 固定旧版本，就必须保留对应版本目录，不能删除。旧版本目录也是回滚的唯一依赖。
- 启动时校验文件名、目录名、前置元数据、版本和 SHA-256，任何不一致都直接失败。
- 已固定版本被原地修改会因哈希不一致导致启动失败；未登记的新技能目录同样会失败。
- 哈希基于规范化后的内容（去 BOM、CRLF→LF），Windows 与 Unix 检出结果一致。
- 不做版本回退：run 固定的版本缺失或哈希不符时，该 run 明确失败，不会悄悄换成当前版本。

常用命令：

```sh
make skills-verify       # 只校验，不写文件
make skills-lock         # 新增/更新锁；已有固定版本不会被静默升级
make skills-lint         # 作者规则检查，只警告
make skills-lint-strict  # 有警告即失败（CI 用）
go run ./cmd/skills -dir skills -write
```

`skills-lock` 在发现更新版本时只给出 warning，仍保持原固定版本；升级必须显式修改 `skills.lock.json`，避免一次编辑意外切换上下文。

## 运行时行为

1. 启动时 `skills.Open` 通过 `LocalSource` 生成 L1 清单（目录 + 版本表），并完成锁文件校验；目录不存在时按无技能启动。
2. `admit` 节点只把技能目录快照写入 `runs.state` 的 `skill_pins`（名称、版本、哈希），不写正文。旧 run 的 `skills` 正文字段继续兼容。
3. 新 run 固定后，该 run 只按自己的 pins 解析正文；期间发生的升级、刷新或回滚都不影响它。
4. research 提示词只列出已固定技能的名称、版本和用途；模型调用 `load_skill` 后，正文以 `<skill>` 形式进入工具消息，不再包 `<external_data>`。
5. 同一 run 最多加载 3 个不同技能；同一技能重复调用命中工具结果缓存。
6. 技能不可用（`not_found`、`version_mismatch`、`hash_mismatch`、`invalid_pin`、`source_error`）时，工具返回带 `name`/`version` 的错误，研究状态标记为 `degraded`，原因为 `skill_unavailable: name@version`；其他工具错误不影响研究继续。

## 缓存分层

- **L1 清单缓存**：内存中的目录与版本表，TTL 30 秒。刷新前先比较本地指纹（目录名、版本目录、文件 mtime/size）；指纹未变只续期，不重扫。
- **刷新语义**：刷新只重建 L1，并预热当前锁中版本的正文；L2 不清空，已缓存的正文按内容哈希继续命中。
- **刷新失败**：保留上一份清单但让 `List` 返回错误，新 run fail-closed；进行中的 run 走 `Resolve`，不受刷新影响。
- **L2 正文缓存**：键为 `name + version + sha256` 的 LRU，最多 64 条 / 4 MiB；并发同键请求合并为一次加载；错误结果不缓存。
- **L3 远程缓存**：仅保留 `RemoteCache` 接口占位，v1.7 不创建实现、不读取 Redis 配置。

## 作者规则（lint）

`make skills-lint` 检查两类问题：

- `body_too_long`：正文超过 500 行，提示拆分或精简。
- `missing_trigger_cue`：description 缺少触发词（当、用于、需要、适合、when、use when、for）。

lint 只给 warning；`-strict` 打开后有任何 warning 即非零退出，供 CI 使用。正文长度、描述长度、数量等硬限制仍由加载和锁校验强制。

## 触发评测（可选）

真实模型是否会在正确场景加载技能，用独立 fixture 验证，不依赖生产 `skills/` 目录：

```sh
make skills-eval
# 等价于：
go run ./cmd/skilleval -dir eval/skills-fixtures -cases eval/skills-cases/cases.json
```

- 只读取 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`，不加载其他配置，也不要求 `DATABASE_URL`。
- fixture 技能放在 `eval/skills-fixtures`，自带 `skills.lock.json`；用例放在 `eval/skills-cases/cases.json`。
- 每个用例使用独立 provider 和 45 秒预算，工具 `search_travel_notes` 固定返回空结果。
- 判定标准是本次加载的技能集合与 `expect_loaded` 完全一致；缺省 CI 不跑，需要凭证时手动执行。

## 升级与回滚

升级：

1. 新建 `skills/<name>/<new-version>/SKILL.md`，修改前置元数据的 `version`。
2. 把 `skills.lock.json` 中的 `version` 改为新版本，并运行 `make skills-lock` 写入新哈希。
3. 运行 `make skills-verify`、`make skills-lint-strict` 和测试；重新部署。
4. 新部署下新 run 使用新版本；旧 run 若仍固定旧版本，保留旧版本目录即可继续运行。

回滚：

1. 把锁中的 `version` 改回旧版本，运行 `make skills-lock`。
2. 无需删除新版本目录。

## 可观测性

- 启动日志打印技能数量、目录、L1/L2 缓存初始状态，并逐条打印 `skill pinned`（名称、版本、SHA-256）。
- `GET /api/meta` 每次请求读取当前 L1 清单，返回 `name`、`version`、`sha256`；不包含 description 和正文。
- 事件流包含 `research.skill_loaded`（名称、版本、哈希）；`research.completed` 的 `skills_loaded` 列出本次 run 实际加载的技能。
- Web 端把连续的技能加载合并为一条「已加载技能 name@version」，不展示正文。
- 公开分享只输出最终方案和研究摘要，run 状态中的 `skill_pins`、`skills` 与正文一律不进入 `/api/share/*` 和日志。
