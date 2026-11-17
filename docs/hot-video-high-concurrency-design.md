# 热点视频高并发设计

> 基于已实现代码（commit 13bd0f4 及后续修复）编写，反映真实落地状态。
> 技术栈：Go + RabbitMQ + Redis + MySQL，API / Worker / Scheduler 三进程。

## 1. 问题定义

热点视频场景下，单个视频在短时间内聚集大量用户互动：

- **写压力**：点赞、评论集中打到同一视频，逐条同步写 MySQL 会产生行锁竞争和连接耗尽。
- **读压力**：视频详情、点赞状态、评论列表被高频读取，缓存失效时大量请求同时回源。
- **计算压力**：互动不断改变热度，每次请求全量聚合排序不可持续。
- **一致性压力**：异步化后消息可能重复、乱序、丢失，需要保证最终一致且不重复计数。

设计目标：把不可控的同步峰值转化为可控的异步批量处理，同时保证幂等、有序、可恢复。

## 2. 整体架构

```text
                         写链路（MQ-first 异步命令）
 Client
   │  Idempotency-Key + 业务参数
   ▼
 API（鉴权、限流、参数校验）
   ├── 生成 operationId / commentId（UUID）
   ├── 集中版本序列 opVersion（Redis INCR，按业务键）
   ├── Redis 幂等记录（SET NX，存完整不可变 Command）
   └── RabbitMQ Publisher Confirm → 202 Accepted
           │
           ▼
 RabbitMQ（feed.interaction.commands Exchange，独立 Queue）
   ├── interaction.command.like.queue（DLX → retry → DLQ）
   └── interaction.command.comment.queue（DLX → retry → DLQ）
           │
           ▼
 Worker（有界消费，手动 ACK）
   ├── MySQL 事务：业务行 + video_stats + interaction_outcomes
   ├── Redis Lua：状态 + 统计 + 热度（按 outcome 重放，幂等）
   └── 缓存失效（评论首页、视频详情）

                         读链路（缓存 + 有界回源）
 Client → API → Redis
   ├── video:detail:{id}（singleflight + TTL jitter + 负缓存）
   ├── video:stats:{id}（绝对值 + stats_version）
   ├── like:state:{vid}:{uid}（liked + op_version）
   ├── comment:first_page:{vid}:{limit}（singleflight + cache-aside）
   └── rank:video:hot:60m → 快照 → MGET 页面组装
           │ 缓存缺失/异常
           ▼
    部分接口使用 singleflight，缓存缺失时回源 MySQL（连接池与并发闸口待补齐）

                         热度链路（独立 Scheduler）
 Worker 写分钟桶 hot:video:minute:{minute}
           │
           ▼
 Scheduler（Redis 分布式锁单活，ownerToken + TTL 续约）
   ├── add：新封盘分钟桶加入聚合 ZSET（Lua 原子 + 检查点）
   ├── remove：过期分钟桶扣减并删除
   └── cleanup：清理低分成员
           │
           ▼
 rank:video:hot:60m → 首页快照（5min TTL）→ offset 分页
```

## 3. 写链路详解

### 3.1 API 接收与幂等

用户每次操作携带 `Idempotency-Key`。API 处理流程：

1. **鉴权与限流**：JWT 鉴权，按用户/接口 Redis 限流。
2. **生成身份**：
   - `operationId`：UUID，全局唯一。
   - `commentId`：评论创建时预生成 UUID（字符串，避免 JS 精度问题）。
   - `opVersion`：Redis 按业务键集中 `INCR` 生成，保证同键单调递增；首次从 1 开始，后续递增。评论创建也走集中序列（非固定值）。
3. **幂等记录**：Redis `SET NX` 保存完整不可变 Command（含指纹、消息字节、路由键）。并发同 Key 时仅一个记录写入成功；其余请求在检测到记录已存在且指纹相同时复用该记录重发（当前实现允许并发请求各自发布 Command，下游通过 outcome/版本去重）。
4. **发布确认**：RabbitMQ 持久化消息 + Publisher Confirm，成功才返回 202；失败/超时返回 503，客户端用原 Key 重试。

Command 模型：

```json
{
  "schemaVersion": 1,
  "operationId": "uuid",
  "idempotencyKey": "客户端幂等键",
  "requestFingerprint": "sha256(操作|用户|视频|内容)",
  "opVersion": "uint64 字符串",
  "type": "like.create | like.cancel | comment.create | comment.delete",
  "actorId": 1,
  "videoId": 100,
  "commentId": "uuid（评论）",
  "content": "评论内容",
  "occurredAt": "业务时间"
}
```

指纹设计：

- 点赞/取消：`sha256(操作|用户|视频)`。
- 评论创建：`sha256(操作|用户|视频|内容)`。
- 评论删除：`sha256(操作|用户|0|commentId)`（含目标评论 ID，videoID 固定为 0 保证重试一致）。

### 3.2 RabbitMQ 可靠性

- **拓扑**：独立 `feed.interaction.commands` Exchange；like/comment 各一个 Queue（`interaction.command.like.queue` / `interaction.command.comment.queue`）；主队列 DLX → retry queue（带 TTL）→ DLQ。
- **发布**：复用 Connection 和 Channel；`publishMu` 串行化发布；Confirm 监听按 deliveryTag 分发；Nack/超时/Channel 关闭均 fail pending。
- **消费**：手动 ACK；`Qos(prefetch)` 限制拉取；Worker 内最多 3 次重试（带 TTL 间隔），非重试错误直接 DLQ。
- **背压**：prefetch + 有界 Worker + 客户端超时，防止下游过载。

### 3.3 Worker 微批次与事务

Worker 消费 Command 后，在 MySQL 事务中完成：

1. **查 outcome**：按 `operationId` 查 `interaction_outcomes`。
   - 已存在且指纹一致：直接返回已有 outcome（跳过业务写入）。
   - 指纹不一致：非重试错误，进 DLQ。
2. **业务写入**：
   - 点赞：事务内 SELECT 现有行后 INSERT/SAVE，仅当 `cmd.opVersion > 现有版本` 才更新状态。
   - 评论创建：按 `commentId` 查重，`INSERT` 新行；重复则跳过。
   - 评论删除：条件软删除（作者 + 未删 + 版本更高）。
3. **统计更新**：`atomicUpdateStatsTx` 用 `INSERT IGNORE + UPDATE` 原子增量，避免 read-modify-write 丢失更新。
4. **写 outcome**：记录本次真实效果（状态、统计绝对值、stats_version、热度增量、热度分钟），与业务变更同事务。

### 3.4 Redis 重放

MySQL 提交后，Worker 按 outcome 调用 Lua 原子脚本：

1. **去重**：`interaction:applied:{operationId}` 存在则跳过。
2. **版本守卫**：`stats_version < 当前` 则跳过。
3. **状态更新**：点赞写 `like:state:{vid}:{uid}` hash（`liked` + `op_version`）；评论无状态变更。
4. **统计覆盖**：写 `video:stats:{vid}` 绝对值（like_count/comment_count/stats_version）。
5. **热度写入**：`heat_delta != 0` 时 `ZINCRBY hot:video:minute:{heat_minute}`。
6. **标记完成**：`SET interaction:applied:{operationId}`。

**重放安全**：MySQL 成功但 Redis 失败时，命令重新投递；Worker 读 outcome（不重复写 MySQL），重放 Lua 脚本（marker 去重），最终一致。

## 4. 读链路详解

### 4.1 视频详情

- 先查负缓存 `video:detail:{id}:empty`（30s TTL），命中直接 404。
- 查缓存 `video:detail:{id}`，命中返回。
- 未命中：`singleflight.Do` 按视频 key 合并并发回源；回源成功写缓存（TTL + 随机抖动 0-30s），404 写负缓存。
- 评论数变化时 Worker 删除详情缓存，触发下次重建。

### 4.2 点赞状态

- 查 `like:state:{vid}:{uid}` hash，返回 `liked + op_version`。
- hash 缺失或过期：回源 MySQL（`LikeStateWithVersion` 一次查询返回状态+版本）。
- 计数读 `video:stats:{vid}`；缺失回源 `video_stats` 表。
- DB 回源有 singleflight 保护。

### 4.3 评论列表

- 首页按 `(videoID, limit)` 缓存 `comment:first_page:{vid}:{limit}`。
- `singleflight` 按同 key 合并回源；回源后写缓存。
- Worker 在评论创建/删除后 SCAN 删除该视频所有 limit 变体。

### 4.4 热榜读取

- 第一页：`ZREVRANGE rank:video:hot:60m 0 199` 取 Top 200，创建快照（5min TTL），返回 `snapshotId + nextOffset`。
- 后续页：`snapshot.GetPage(snapshotID, offset)` 直接读快照，不查实时榜。
- 页面组装：`MGET video:detail:{id}...` 批量读缓存；缺失 ID 批量回源 MySQL 并回填；按快照 ID 顺序组装。
- 快照过期：创建新快照，前端替换列表（不追加）。

## 5. 热度链路详解

### 5.1 计分规则

- 点赞真实变化（未点赞→点赞）：+5，写入命令业务时间所属分钟桶。
- 评论真实新增：+3，写入命令业务时间所属分钟桶。
- 取消点赞：不新增活动分，原互动随分钟桶自然过期。
- 重复命令、旧版本：不产生热度。窗口外迟到命令仍会写入原分钟桶，但不保证进入聚合榜（封盘后迟到分值尚无修正机制）。

### 5.2 滑动窗口

- 分钟桶：`hot:video:minute:{yyyyMMddHHmm}`，ZSET 按视频记分。
- Scheduler 每分钟执行：
  - add：将 `now-1min` 的封盘桶加入 `rank:video:hot:60m`。
  - remove：将 `now-61min` 的桶从聚合 ZSET 扣减并删除。
- 每 5 分钟清理低分成员（score ≤ 0）并限制榜单规模。

### 5.3 Scheduler 单活与幂等

**为什么需要单活**：Worker 扩容为多实例后，若每个实例都跑定时聚合，同一分钟桶会被重复加分/扣分。

**实现**：独立 `cmd/scheduler` 进程，Redis 分布式锁保证单活：

- 锁：`lock:hot-rank:scheduler`，`SET NX PX` 获取，value 为随机 `ownerToken`。
- 续约：Lua 校验 token 后 `PEXPIRE`；续约失败停止调度。
- 释放：Lua 校验 token 后 `DEL`，防止误删他人锁。
- 崩溃：锁 TTL 过期后，依赖外部重启或新启动实例接管（当前无常驻 Standby 循环）。

**分钟幂等**：Lua 脚本原子完成：

1. 校验锁 ownerToken。
2. 检查 `hot:window:applied:{add|remove}:{minute}` 检查点，已处理则跳过。
3. remove 方向额外检查 `add` 检查点是否存在（防止从未添加的桶被扣分）。
4. 遍历分钟桶成员，`ZINCRBY` 聚合 ZSET。
5. 写检查点 + 更新高水位游标 `hot:window:cursor:{add|remove}`。

**补跑**：Scheduler 崩溃或停机后，从高水位游标下一分钟开始，按配置每轮最多补跑 N 个连续分钟，直到追上当前时间。

## 6. 数据库设计

### 6.1 likes（点赞关系）

```sql
id           BIGINT UNSIGNED AUTO_INCREMENT
user_id      BIGINT UNSIGNED NOT NULL
video_id     BIGINT UNSIGNED NOT NULL
op_version   BIGINT UNSIGNED NOT NULL DEFAULT 0  -- 操作版本，防乱序
created_at   DATETIME(3)
updated_at   DATETIME(3)
deleted_at   DATETIME(3) NULL

UNIQUE KEY uk_likes_user_video (user_id, video_id)  -- 幂等基础
KEY idx_likes_video_deleted (video_id, deleted_at)
KEY idx_likes_video_version (video_id, op_version)
```

### 6.2 comments（评论）

```sql
id                   VARCHAR(36) NOT NULL        -- UUID 字符串，API 预生成
user_id              BIGINT UNSIGNED NOT NULL
video_id             BIGINT UNSIGNED NOT NULL
content              VARCHAR(500) NOT NULL
op_version           BIGINT UNSIGNED NOT NULL DEFAULT 0
operation_id         VARCHAR(64) NULL            -- 创建命令身份
request_fingerprint  VARCHAR(128) NOT NULL DEFAULT ''
created_at           DATETIME(3)
updated_at           DATETIME(3)
deleted_at           DATETIME(3) NULL

PRIMARY KEY (id)
UNIQUE KEY uk_comments_operation (operation_id)   -- 防重复创建
KEY idx_comments_video_created (video_id, created_at, id)
```

### 6.3 video_stats（视频统计）

```sql
video_id       BIGINT UNSIGNED NOT NULL
like_count     BIGINT NOT NULL DEFAULT 0
comment_count  BIGINT NOT NULL DEFAULT 0
stats_version  BIGINT UNSIGNED NOT NULL DEFAULT 0  -- 防旧值覆盖新值
updated_at     DATETIME(3)

PRIMARY KEY (video_id)
```

更新方式：`INSERT IGNORE` 建行 + `UPDATE ... SET count = count + delta, stats_version = stats_version + 1`，原子增量，无 read-modify-write 竞争。

### 6.4 interaction_outcomes（操作结果，重放依据）

```sql
operation_id         VARCHAR(64) NOT NULL   -- 命令唯一身份
request_fingerprint  VARCHAR(128) NOT NULL  -- 防指纹冲突
command_type         VARCHAR(32) NOT NULL
business_key         VARCHAR(128) NOT NULL  -- like:uid:vid / comment:cid
op_version           BIGINT UNSIGNED NOT NULL
video_id             BIGINT UNSIGNED NOT NULL
db_outcome           VARCHAR(32) NOT NULL   -- applied / unchanged / duplicate
result_state         VARCHAR(64) NOT NULL
like_count           BIGINT NOT NULL        -- 提交时统计绝对值
comment_count        BIGINT NOT NULL
stats_version        BIGINT UNSIGNED NOT NULL
heat_delta           BIGINT NOT NULL        -- 本次热度增量
heat_minute          VARCHAR(16) NOT NULL   -- 热度所属分钟
created_at           DATETIME(3)

PRIMARY KEY (operation_id)
KEY idx_outcomes_heat_minute (heat_minute)
KEY idx_outcomes_business_key (business_key)
```

**作用**：MySQL 提交后 Redis 失败时，Worker 重放不重新计算业务效果，而是读 outcome 恢复 Redis 状态。这是保证"至少一次投递 + 幂等"的核心。

## 7. Redis 数据结构

| Key | 类型 | 用途 |
|-----|------|------|
| `idem:{sha256}` | String | 幂等记录（完整 Command + 指纹 + 消息字节） |
| `seq:like:{uid}:{vid}` | String | 点赞操作版本序列 |
| `seq:comment:{cid}` | String | 评论操作版本序列 |
| `like:state:{vid}:{uid}` | Hash | `liked` + `op_version` |
| `video:stats:{vid}` | Hash | `like_count` / `comment_count` / `stats_version` |
| `video:detail:{vid}` | String(JSON) | 视频元信息 + 评论数 |
| `video:detail:{vid}:empty` | String | 负缓存（404 防穿透） |
| `comment:first_page:{vid}:{limit}` | String(JSON) | 评论首页 |
| `comment:detail:{cid}` | String(JSON) | 评论详情 |
| `hot:video:minute:{minute}` | ZSET | 分钟热度桶 |
| `rank:video:hot:60m` | ZSET | 60 分钟聚合热榜 |
| `feed:hot:snapshot:{id}` | String(JSON) | 热榜分页快照 |
| `interaction:applied:{opId}` | String | Redis 重放去重标记 |
| `hot:window:applied:{add\|remove}:{minute}` | String | 分钟聚合检查点 |
| `hot:window:cursor:{add\|remove}` | String | 聚合高水位游标 |
| `lock:hot-rank:scheduler` | String | Scheduler 单活锁 |

## 8. 故障恢复设计

### 8.1 各组件故障语义

**RabbitMQ 不可用**
- API 发布失败/超时：返回 503，客户端用原 Idempotency-Key 重试。
- 不返回虚假 Accepted。
- 读接口继续服务。

**MySQL 不可用**
- Worker 事务失败：有界重试（3 次，间隔 TTL），最终 Nack → retry queue → DLQ。
- 不无限 requeue，不 tight loop。
- 读接口优先缓存，回源有 singleflight 限制。

**Redis 不可用**
- 写接口：操作版本/幂等记录依赖 Redis，fail-closed 返回 503。
- 读接口：部分接口缓存失效时使用 singleflight 回源；视频详情有负缓存防穿透。
- 不切换到逐请求同步写 MySQL（避免故障转为数据库风暴）。

**Worker 崩溃**
- 未 ACK 消息重新投递。
- 业务唯一键 + opVersion + outcome 保证重复执行不重复计数。
- 重启后继续消费，不丢命令。

**Scheduler 崩溃**
- Redis 锁 TTL 过期，依赖外部重启或新启动实例接管。
- 从分钟检查点和高水位游标补跑，不重复聚合。
- 接管期间 Worker 继续写分钟桶，不受影响。

### 8.2 关键恢复场景

**MySQL 提交后 Redis 失败**
```
Worker 事务提交（likes + video_stats + outcome）
  → Redis Lua 失败
  → 消息重新投递
  → Worker 读 outcome（不重复写 MySQL）
  → 重放 Lua（marker 去重，stats_version 防旧覆盖）
  → 最终一致
```

**Redis 成功后 ACK 前崩溃**
```
Redis Lua 已执行（含 marker）
  → Worker 崩溃，消息未 ACK
  → 消息重新投递
  → Worker 读 outcome，重放 Lua
  → marker 已存在，跳过，不重复效果
```

**重复/乱序消息**
```
同一 Command 多次投递：
  → outcome 已存在 → 直接返回，不重复写 MySQL

旧版本 Command 晚到：
  → MySQL op_version 比较 → 拒绝旧版本（不产生业务变更）
  → Redis stats_version 严格小于当前才跳过；相同版本下点赞状态可能按最新 outcome 覆盖
```

**Scheduler 分段中崩溃**
```
Scheduler 在遍历分钟桶中途崩溃
  → 锁 TTL 过期，依赖外部重启或新启动实例接管
  → 检查点未写（未完成），重新处理该分钟桶
  → 已写检查点的分钟不重复处理
```

**MQ 发布失败**
```
Publisher Confirm 失败/超时/Nack
  → API 返回 503，不返回 202
  → 客户端用原 Idempotency-Key 重试
  → 幂等记录仍在，重试复用原 Command
```

### 8.3 对账与修复

- 迁移 006 提供 `video_stats` 回填 SQL，从 likes/comments 实际数据重建统计。
- 历史漂移可通过停写 → 排空队列 → 回填 → 对账修复。
- `interaction_outcomes` 首版不清理，保留完整重放证据。

## 9. 已验证的性能与一致性

本地环境（Docker MySQL/Redis/RabbitMQ + 三进程）实测：

- **写入**：400-480 Accepted，0 传输错误，约 27-48 QPS（受单热点行锁与限流约束）。
- **读取**：约 7,000 次热榜/详情读取，P50 ≈ 4-7ms，P95 ≈ 20-30ms。
- **背压**：大量请求触发 429 限流，系统未崩溃。
- **一致性**：压测后 `video_stats` 与 `likes`/`comments` 实际数据完全一致，无丢失更新、无重复计数。
- **幂等**：同 Key 重试返回相同 operationId；重复评论只产生一行；删除重试不重复减计数。

## 10. 当前边界与后续扩展

**未实现（后续扩展）**：
- 点赞集合分片（Big Key 时启用，当前数据量不需要）。
- 迟到桶修正（`hot:video:late:{minute}`，Scheduler 聚合期间的迟到互动）。
- Redis 全量丢失自动恢复（当前 fail-closed，需人工介入）。
- 故障注入自动化（5 个断点的脚本化验证）。

**注意**：迟到命令的热度当前直接写入命令业务时间所属分钟桶，未做窗口外拦截；过期桶由 Scheduler 自然清理。

**不引入**：
- Kafka / Flink / etcd / Raft / 跨地域多活。
- 分布式 Exactly Once 语义（用至少一次 + 幂等替代）。
