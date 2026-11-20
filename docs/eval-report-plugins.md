# TripPilot 评测报告

- 生成时间：2026-11-20T12:44:08+08:00
- 用例数：8

## 运行环境

- 插件开关：HOTEL_PLUGIN_ENABLED=true，GUIDE_PLUGIN_ENABLED=true
- 酒店 sidecar：http://127.0.0.1:8090（healthz 通过）
- 攻略搜索：https://api.deepseek.com
- 研究时限：12s；研究工具调用上限：6
- 命令：go run ./cmd/eval -plugins -cases eval/cases-plugins -output docs/eval-report-plugins.md

说明：
- 本报告使用规格默认 12s 研究时限。真实 sidecar 与搜索链路下研究简报 LLM 经常在截止前未返回，因此 8 条均记录为 `degraded`；工具结果仍被保留：酒店使用确定性回退选择首个有价结果并计入住宿，避免预算丢失。
- 本环境对 `site:xiaohongshu.com` 的搜索返回 `empty_result`（约 250ms），因此攻略命中数为 0；攻略加权与引用路径由 fake ToolLLM 单测和集成测试覆盖，真实搜索供应商结果依赖外部索引。
- `p3-hotel-missing` 停在 `awaiting_user` 属于预期行为；该用例按设计不生成版本。

| 指标 | 值 |
| --- | --- |
| 硬约束满足率 | 100.0% |
| 通过用例 | 8/8 |
| 平均修复轮数 | 0.62 |
| 3 天 p95 延迟 | 38522 ms |
| POI 有效率 | 87.5% |
| JSON 合法率 | 100.0% |
| 人工评分 | 待填 |

| 用例 | 结果 | 硬约束 | 修复轮数 | 延迟 | POI | JSON | 研究 | 研究耗时 | 工具调用 | 住宿 | 攻略匹配 | 说明 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| p1-hotel-anchor | 通过 | 是 | 0 | 21021 ms | 是 | 100.0% | degraded | 12000 ms | 5 | 500 | 0 | - |
| p1-hotel-budget | 通过 | 是 | 1 | 38522 ms | 是 | 100.0% | degraded | 12001 ms | 4 | 800 | 0 | - |
| p1-hotel-specified | 通过 | 是 | 1 | 25772 ms | 是 | 100.0% | degraded | 12001 ms | 4 | 700 | 0 | - |
| p2-guide-hutong | 通过 | 是 | 0 | 19766 ms | 是 | 100.0% | degraded | 12000 ms | 4 | 0 | 0 | - |
| p2-guide-no-match | 通过 | 是 | 1 | 25019 ms | 是 | 100.0% | degraded | 12002 ms | 4 | 0 | 0 | - |
| p3-hotel-missing | 通过 | 否 | 0 | 14504 ms | 否 | 0.0% | degraded | 12000 ms | 4 | 0 | 0 | 按预期等待用户确认：hotel_unconfirmed 未确认你指定的酒店：不存在的测试酒店 |
| p4-guide-timeout | 通过 | 是 | 1 | 20272 ms | 是 | 100.0% | degraded | 8162 ms | 6 | 0 | 0 | - |
| p4-tool-timeout | 通过 | 是 | 1 | 26526 ms | 是 | 100.0% | degraded | 9188 ms | 5 | 0 | 0 | - |
