# TripPilot

多约束旅行规划 Agent。首版聚焦北京 1～4 天行程：自然语言输入、确定性校验、自动修复、版本调整和分享。

## 状态

v1 MVP 开发中。四份规格文档已批准，按任务清单逐步实现。

## 文档

- [Spec](docs/mew-spec/v1-mvp/spec.md)
- [Plan](docs/mew-spec/v1-mvp/plan.md)
- [Tasks](docs/mew-spec/v1-mvp/task.md)
- [Checklist](docs/mew-spec/v1-mvp/checklist.md)
- [系统架构](docs/architecture/系统架构设计.md)
- [产品需求](docs/prd/项目立项.md)

## API 准备

本地开发需要：

- OpenAI 兼容 LLM API Key
- 高德 Web 服务 Key（服务端）
- 高德 JS API Key + 安全密钥（前端）
- PostgreSQL 16（Docker 即可）

复制 `.env.example` 为 `.env` 并填入凭证。不要提交 `.env`。

## 快速开始

开发中，后续补全 Compose 启动、迁移、种子数据和评测命令。
