# 热点视频高并发：验证、压测与前端适配 Spec

> 状态：Implemented。四个阶段全部完成并通过 review：阶段一（迁移+读模型补丁）、阶段二（E2E 验证）、阶段三（前端适配）、阶段四（压测工具+实测）。故障注入 §3.3 为人工后续。  
> 上游：`.agent/hot-video-high-concurrency-spec.md`（后端 A-E 已实现并提交 13bd0f4）  
> 范围：剩余 3 项任务——端到端验证、压测与故障注入、前端 pending 适配。  
> 不包含：运行文档补充（用户明确暂缓）。

## 1. 背景与当前状态

后端已完成 Milestone A-E 并提交，但存在三个尚未闭环的风险：

1. **没有真实环境验证过**：迁移 005、三进程启动、MQ 拓扑声明、Outcome 消费、Scheduler 聚合都只有编译和 miniredis 单测覆盖，未在真实 MySQL/Redis/RabbitMQ 上跑通。
2. **前端仍是旧契约**：`frontend/src/api/client.ts` 点赞/评论无 `Idempotency-Key`，评论 ID 为 `number`，删除期望 `{deleted:boolean}`，热榜分页仍用 `lastVideoId`；后端已改为 202 + pending + 字符串评论 ID + `offset` 分页。
3. **没有性能证据**：Spec §15.3 的压测场景未落地，不能声称任何容量指标。

## 2. 任务一：本地端到端验证

### 2.1 验证顺序（评审修正）

严格按以下顺序，不得跳步：

```text
迁移演练（空库 + 含旧数据库）
  → 兼容消费者与 MQ 拓扑就绪
  → API E2E
  → 前端验收
  → 压测
```

### 2.2 迁移演练（新增，阻塞项）

- 005 当前只建 `video_stats` 表，**不回填旧数据统计**，旧视频会从零计数。必须补：
  - 停写 → 排空旧队列 → 按 likes/comments 实际数据回填 `video_stats` → 对账。
  - 回填 SQL 或脚本需在 Spec 中明确，执行后抽查若干视频与 `COUNT(*)` 一致。
- 验证 `comments.id` 自增转字符串：旧数值 ID 保留、软删除记录保留、主键不丢。
- 验证 DDL 锁表影响与部分失败续跑方式。
- 增加回滚备份：`CREATE TABLE comments_backup_005 AS SELECT * FROM comments`（执行 MODIFY 前）。
- 环境启动：`docker compose up -d`，分别启动 `cmd/api`、`cmd/worker`、`cmd/scheduler`，确认 Worker 声明新旧队列成功、Scheduler 获锁 active。

### 2.3 功能验证场景

用 Apifox 验证（不用 curl 优先）：

1. 注册/登录拿 token。
2. 发布视频（本地上传或直填 URL）。
3. 点赞/取消点赞：
   - 带 `Idempotency-Key` 返回 202 + `pending` + `operationId` + `opVersion`。
   - 同 Key 重试返回同一 `operationId`，不产生第二条 Command。
   - 不同内容同 Key 返回 409。
   - Worker 消费后 `like-status` 读到新状态与计数；MySQL `likes` 与 `interaction_outcomes` 有记录。
4. 评论创建/删除：
   - 创建返回 202 + pending 评论（字符串 ID）。
   - pending 期间删除返回 409 `comment_not_ready`。
   - Worker 消费后评论列表可见；删除后消失；`video_stats.comment_count` 正确。
5. 热榜：
   - 互动后 `hot:video:minute:*` 有分值，Scheduler 聚合到 `rank:video:hot:60m`。
   - 第一页返回 `snapshotId`，后续用 `snapshotId + offset` 翻页；同一快照内无重复/遗漏；快照过期后明确开新快照。
6. 幂等与故障：
   - 点赞后立刻刷新，前端 pending 语义可解释（短暂旧值属预期）。
   - 重启 Worker 后未 ACK 消息能重新消费且不重复计数。

### 2.4 验收标准

- 上述场景全部在真实环境走通，无 500、无数据丢失、无重复计数。
- 发现的问题按 P0/P1 修复后再验证，不直接压测。

## 3. 任务二：压测与故障注入脚本

### 3.1 工具选择（已收敛）

**选择 Go 独立 HTTP 驱动**（`backend/cmd/loadtest`），不维护双工具。

硬性约束：

- 只通过公开 HTTP API 驱动负载，**不得复用内部 Command/ID 生成逻辑绕过 API**，否则测不到真实链路。
- 固定负载模型：预置用户池、明确限流是否开启、客户端超时、预热时间、分位数统计口径。
- **分别报告两个指标**：API Accepted 吞吐/延迟，与 Worker 最终提交延迟（从 Accepted 到 outcome 落库）。
- 脚本参数化：并发数、时长、目标视频 ID、用户池大小。
- 结果输出 JSON + 简短结论，纳入 git 管理（放 `polish/`）。

### 3.2 压测场景

1. **单热点点赞风暴**：N 个用户对同一视频高频点赞/取消，观察 API P95/P99、MQ 积压、Worker outcome 写入、MySQL 实际写入次数、最终计数一致性。
2. **单热点评论风暴**：N 个用户评论同一视频，观察批量 INSERT 行为、评论数一致性、首页缓存是否反复失效。
3. **缓存失效读风暴**：主动删除视频详情缓存后并发读取，验证 singleflight/负缓存下 MySQL 回源次数受控。
4. **热榜混合流量**：持续互动 + 并发热榜多页读取，验证快照分页稳定、Scheduler 不重复聚合。

### 3.3 故障注入（最小必要集）

必测断点：

- **MySQL 提交后 Redis 失败**：重放只读 outcome，不重复计数。
- **Redis 成功后 ACK 前崩溃**：消息重新投递，operationId 去重。
- **重复/乱序消息**：手工重放同一 Command，验证 outcome/版本去重。
- **Scheduler 分段中崩溃**：接管后从分段游标继续，不重复聚合。
- **MQ 发布失败**：不返回 202。
- Redis/MySQL 短断：验证有界失败与恢复，不 tight loop。

Redis 全量丢失恢复属于独立 runbook 范围，本期不做自动化。

### 3.4 指标与报告

- 记录：环境、数据量、并发模型、QPS、P95/P99、错误率、MQ 最老消息年龄、MySQL 写入次数、Redis 命中率。
- 输出到 `polish/` 或 `.agent/` 下的结果文件（JSON + 简短结论），不编造提升倍数。
- 脚本需可重复运行、参数化（并发数、时长、目标视频 ID）。

## 4. 任务三：前端 pending 适配

### 4.0 前置后端修复（评审发现，阻塞前端）

当前读契约无法支持“版本追平清 pending”，先补后端：

- `LikeStatus` 读路径（`Status()` 的 cache 与 DB 两个分支）必须返回当前 `opVersion`，否则前端永远不知道何时清除 pending。
- `Comment.OpVersion` 当前为 `json:"-"`，评论读模型需暴露版本（或提供等价已提交标记）。
- 以上属于小范围后端补丁，随前端适配同批提交。

### 4.1 API 层（`frontend/src/api/client.ts`）

- **修复 headers 覆盖 bug**：`request` 中 `...options` 会覆盖已合并的 headers，需把 `...options` 展开放在 headers 之前或单独合并。
- `request` 支持可选 `Idempotency-Key` 注入。
- 点赞/取消/评论创建/删除：生成并复用 `Idempotency-Key`（`crypto.randomUUID()`，失败时降级）。
- **Key 由用户意图持有**：超时/503 用原 Key 重试，新操作才生成新 Key。
- `Comment.id` 改 `string`；`deleteComment` 参数改 `string`；返回值改 pending 结构。
- `LikeStatus` 增加 `status?/operationId?/opVersion?`。
- `listHotVideos` 参数改 `offset`，响应增加 `nextOffset`。
- **版本比较用 BigInt**（opVersion 为字符串，避免超 JS 安全整数）。
- 保留 HTTP 状态与错误码，409/503 分别处理。

### 4.2 页面层

- `VideoDetailPage.vue`：
  - 点赞点击后立即乐观展示目标状态，记录 pending `operationId/opVersion`；刷新状态读到不低于该版本后清除 pending。
  - 评论创建返回 pending 评论插入列表顶部，标记 pending 且禁用删除；Worker 提交后替换为正式评论。
  - 删除 pending 评论被禁用时给出提示；已提交评论删除后标记 pending，刷新后移除。
  - 幂等键在“一次用户意图”内复用；新操作生成新 Key。
- `HotFeedPage.vue`：
  - 分页改用 `snapshotId + offset`，结束依据用 `hasMore`。
  - **快照更换时必须替换列表而非追加**，避免旧快照残留。
  - 后端已忽略 `lastVideoId`，前后端同步发布，不再保留旧游标兼容（本地项目无线上存量）。
- 错误处理：409 幂等冲突提示刷新；503 提示稍后重试且保留原 Key。

### 4.3 验收

- 点赞/评论全流程在 UI 上无重复提交、无 phantom 评论、pending 状态可解释。
- 热榜翻页无重复/遗漏。
- `npm run build` 通过。

## 5. 实施顺序与依赖

```text
任务一 E2E 验证（发现即修）
  → 任务三前端适配（可并行，但验收依赖任务一）
  → 任务二压测/故障注入（依赖任务一环境）
```

先验证再压测，避免把已知功能问题带进性能结论。

## 6. 评审记录

### 6.1 执行方式

- 内置 `oracle`/`reviewer` 因子模型 5 小时配额限制（429）无法运行。
- 降级为外部 `codex-exec` 顾问 + 主 agent 代码核实（degraded mode）。
- 有效评审 1 轮即收敛，无剩余实质分歧。
- 外部顾问 Run：`ef4d09ac-2e9d-4460-86d0-d288ec21ed9f`（结论 BLOCK，提出 5 项修正）。

### 6.2 采纳的修正

1. 验证顺序改为迁移演练→消费者就绪→API E2E→前端→压测。
2. 补 005 的 video_stats 旧数据回填与 comments 回滚备份。
3. 压测选 Go 独立 HTTP 驱动，不复用内部 Command，分别报告 Accepted 与最终提交延迟。
4. 前端前置后端修复：Status/Comment 读模型暴露 opVersion；修 client.ts headers 覆盖 bug。
5. 分页快照更换替换列表、hasMore 判结束、不再保留 lastVideoId。
6. 故障注入聚焦 5 个断点，Redis 全量丢失恢复移出本期。

### 6.3 主 agent 核实

以下发现均已直接读码证实：

- `Status()` 读路径不返回 opVersion（`service.go` statusFromCache/statusFromDB）。
- `Comment.OpVersion` 为 `json:"-"`。
- `client.ts` 的 `...options` 覆盖 headers。
- 005 无 video_stats 回填、无 comments 备份表。

性能收益仍无定量结论，须由任务二实测。
