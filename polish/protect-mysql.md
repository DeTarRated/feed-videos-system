# 高并发点赞保护 MySQL 设计

## 背景

点赞是典型的高频写场景。随着用户量和互动量上升，如果每次点赞都直接打到 MySQL，数据库会承受较大的瞬时写压力，也会放大重复点击、消息重投和统计更新带来的不一致风险。

因此需要把点赞链路设计成“削峰、幂等、可批量落库、可恢复”的形态。

## 架构定位

推荐的目标架构是：

```text
HTTP API -> RabbitMQ -> Worker -> Redis 实时状态 -> MySQL 批量持久化
```

也可以概括为：

```text
MQ 削峰 + Redis 实时状态 + MySQL 批量持久化
```

这里要明确三层职责：

- MQ 负责削峰和异步解耦。
- Redis 负责实时点赞状态、实时计数和幂等控制。
- MySQL 负责最终持久化，不承接每次实时写入。

更准确的表述不是“三级缓存”，而是：

```text
前端乐观状态 / Redis 实时状态 / MySQL 最终持久化
```

## 点赞请求链路

### 1. 用户发起点赞

```text
POST /videos/:id/like
Authorization: Bearer <token>
```

接口层完成必要校验：

- 用户是否登录。
- 视频 ID 是否合法。
- 请求是否明显非法或重复。
- MQ 是否可用。

通过校验后，接口不直接写 MySQL 点赞表，而是投递点赞事件：

```text
like.requested
```

前端可以立即做乐观更新：

- 点赞状态切换为已点赞。
- 点赞数 `+1`。

如果更强调受理语义，也可以返回 HTTP 202。

### 2. RabbitMQ 削峰

RabbitMQ 保存点赞请求事件，例如：

```json
{
  "eventId": "01J4Y6R7Z8ABCDEF1234567890",
  "type": "like.requested",
  "userId": 12,
  "videoId": 1001,
  "delta": 1,
  "occurredAt": "2026-07-30T14:30:20Z"
}
```

取消点赞事件例如：

```json
{
  "eventId": "01J4Y6R7Z8ABCDEF1234567891",
  "type": "like.cancel_requested",
  "userId": 12,
  "videoId": 1001,
  "delta": -1,
  "occurredAt": "2026-07-30T14:31:10Z"
}
```

MQ 的作用是：

- 接住高并发请求。
- 把瞬时写流量变成可消费队列。
- 给 Worker 留出异步处理空间。
- 给重试和死信队列留出接口。

### 3. Worker 消费事件

Worker 消费 `like.requested` 后，先更新 Redis，再把需要持久化的数据打上脏标记。

点赞时：

```text
SADD like:video:{videoId}:users {userId}
SADD like:user:{userId}:videos {videoId}
HINCRBY video:stats:{videoId} like_count 1
SADD like:dirty:videos {videoId}
```

取消点赞时：

```text
SREM like:video:{videoId}:users {userId}
SREM like:user:{userId}:videos {videoId}
HINCRBY video:stats:{videoId} like_count -1
SADD like:dirty:videos {videoId}
```

实际处理必须依赖 `SADD` / `SREM` 的返回值，避免重复消息导致重复加减。

## Redis Key 设计

### 视频点赞用户集合

```text
like:video:{videoId}:users
```

类型：Set。

用途：保存点赞某个视频的用户 ID。

### 用户点赞视频集合

```text
like:user:{userId}:videos
```

类型：Set。

用途：保存某个用户点赞过的视频 ID。

### 视频实时统计

```text
video:stats:{videoId}
```

类型：Hash。

用途：保存视频实时统计，常见字段：

```text
like_count
comment_count
play_count
hot_score
```

### 脏视频集合

```text
like:dirty:videos
```

类型：Set。

用途：记录哪些视频需要异步刷回 MySQL。

### 事件去重

```text
like:event:processed:{eventId}
```

类型：String。

用途：记录某个事件是否已经处理，防止 MQ 重投造成重复消费。

## 幂等控制

点赞幂等的核心是先判断集合变化，再决定是否更新计数。

新增点赞：

```text
added = SADD like:video:{videoId}:users {userId}
if added == 1:
    SADD like:user:{userId}:videos {videoId}
    HINCRBY video:stats:{videoId} like_count 1
    SADD like:dirty:videos {videoId}
```

取消点赞：

```text
removed = SREM like:video:{videoId}:users {userId}
if removed == 1:
    SREM like:user:{userId}:videos {videoId}
    HINCRBY video:stats:{videoId} like_count -1
    SADD like:dirty:videos {videoId}
```

这样可以保证：

- 重复点赞不会重复加数。
- 重复取消点赞不会重复减数。
- MQ 重投不会破坏统计。

## MySQL 批量持久化

MySQL 只负责最终落库，不负责每次实时点赞写入。

### 只同步计数

如果只要求视频点赞数最终准确，可以定时同步：

```text
video:stats:{videoId}.like_count -> video_stats.like_count
```

流程：

1. 从 `like:dirty:videos` 取一批视频 ID。
2. 读取 Redis 中的点赞计数。
3. 批量更新 `video_stats.like_count`。
4. 清理已处理的脏标记。

### 同步点赞关系

如果还要在 MySQL 中保留完整点赞关系，可以按视频全量对齐，或通过增量日志批量 upsert。

全量对齐的思路：

1. 读取视频的 Redis 点赞用户集合。
2. 和 MySQL 中该视频的点赞关系做 diff。
3. 批量新增和软删除。
4. 顺带更新统计值。

## 查询读路径

查询侧应该优先读 Redis，而不是直接只看 MySQL。

### 点赞数

优先读取：

```text
video:stats:{videoId}.like_count
```

Redis 未命中时再回源 MySQL。

### 当前用户是否点赞

优先读取：

```text
SISMEMBER like:video:{videoId}:users {userId}
```

### 我的喜欢列表

优先读取：

```text
SMEMBERS like:user:{userId}:videos
```

再批量查视频基础信息。

## 前端乐观更新

点赞成功后前端可以立即：

- 点赞状态切换。
- 点赞数加一。
- 本地记录该视频已点赞。

如果后续查询结果和本地乐观状态不一致，以服务端状态为准。

## 失败处理

### MQ 写入失败

MQ 写入失败时不能假装成功，否则请求会丢失。推荐返回错误，让前端回滚乐观状态。

### Worker 处理失败

Worker 处理失败时：

1. 短暂错误可以重试。
2. 多次失败进入死信队列。
3. 保留完整事件体，方便人工排查和补偿。

### Redis 不可用

Redis 是实时状态层，如果 Redis 不可用，点赞链路不应继续无保护地把所有流量切回 MySQL。可以返回错误、强限流降级，或作为受控 fallback。

### MySQL 批量落库失败

批量落库失败时：

- 不清理 `like:dirty:videos`。
- 记录错误日志。
- 下次任务继续重试。
- 达到阈值后报警。

## 一致性模型

这个方案是最终一致，而不是强一致。

用户点赞后：

- 前端先看到乐观更新。
- Worker 很快把 Redis 状态改成真实状态。
- MySQL 通过批量任务稍后落库。

短时间内 MySQL 可能落后于 Redis，因此查询侧不能再把 MySQL 当作实时状态来源。

## Outbox 设计

如果还想进一步降低“数据库写成功但 MQ 发布失败”的风险，可以引入 Outbox 模式。

### 设计目标

把业务写入和事件发送放到同一笔 MySQL 事务里，避免事件丢失。

### 表结构

```text
outbox_events
```

字段示例：

```text
id
aggregate_type
aggregate_id
event_type
payload
status
retry_count
next_retry_at
created_at
updated_at
published_at
```

### 写入流程

```text
1. 开启 MySQL 事务
2. 写 likes / comments / follows 等业务表
3. 写 outbox_events
4. 提交事务
```

### 发布流程

```text
1. relay 扫描 pending 事件
2. 发布到 RabbitMQ
3. 成功后标记 published
4. 失败后保留 pending 并重试
```

## 完整链路

### 方案 A：削峰版

```text
1. 前端点击点赞
2. HTTP 接口校验登录态和参数
3. 事件写入 RabbitMQ
4. Worker 消费事件
5. Worker 更新 Redis 实时状态、热度或统计入口
6. 后台任务把 Redis 中的脏数据同步回 MySQL
7. 查询接口优先读 Redis，必要时回源 MySQL
```

### 方案 B：Outbox 增强版

```text
1. 前端点击点赞
2. HTTP 接口校验登录态和参数
3. 同一个 MySQL 事务里写业务表和 outbox_events
4. relay 扫描 outbox_events 并发布到 RabbitMQ
5. Worker 消费事件
6. Worker 更新 Redis 实时状态、热度或统计入口
7. 后台任务把 Redis 中的脏数据同步回 MySQL
8. 查询接口优先读 Redis，必要时回源 MySQL
```

### 方案 C：最终形态

```text
1. 前端点击点赞并做乐观更新
2. MQ 削峰
3. Worker 先更新 Redis 实时状态和计数
4. Redis 用 Set / Hash / dirty set 保证幂等与刷库
5. 定时任务把 Redis 状态批量落回 MySQL
6. 查询侧优先读 Redis，MySQL 作为最终持久化存储
```

## 面试讲解总结

可以这样讲：

> 我会把点赞链路设计成 MQ 削峰 + Redis 实时状态 + MySQL 批量持久化。接口收到请求后先做必要校验并投递事件，Worker 通过 Redis Set 保证点赞关系幂等，用 Hash 维护实时计数，再通过 dirty set 把需要落库的视频批量同步到 MySQL。查询侧优先读 Redis，MySQL 只负责最终持久化。为了避免业务写成功但消息丢失，还可以引入 Outbox，把业务写入和事件发送放到同一个事务里。

一句话概括：

```text
用 MQ 削峰，用 Redis 扛实时状态，用 MySQL 做最终落库。
```
