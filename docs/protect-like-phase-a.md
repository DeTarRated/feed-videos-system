# 方案 A：点赞削峰实施文档

## 目标

方案 A 的目标是把点赞请求从“直接写 MySQL”改成“MQ 削峰 + Worker 更新 Redis + 定时刷回 MySQL”的结构，优先保护数据库写压力，同时保证点赞状态、计数和查询路径可解释、可验证。

本阶段只做点赞 / 取消点赞，不扩展到评论和关注。

## 范围

### 做什么

- 点赞请求投递到 RabbitMQ。
- Worker 消费点赞事件并更新 Redis 实时状态。
- Redis 保存点赞关系、点赞计数和脏数据标记。
- 后台刷库任务把 Redis 中的点赞计数同步回 MySQL。
- 查询侧优先读 Redis，必要时回源 MySQL。

### 不做什么

- 不引入 Outbox。
- 不改评论和关注链路。
- 不做完整增量日志落库。
- 不把整个社交模块都迁到异步。

## 点赞请求链路

```text
1. 前端点击点赞
2. HTTP 接口校验登录态和参数
3. 点赞事件写入 RabbitMQ
4. Worker 消费事件
5. Worker 更新 Redis 实时状态和计数
6. Worker 把对应视频写入 dirty set
7. 刷库任务把 Redis 点赞计数同步回 MySQL
8. 查询侧优先读 Redis，必要时回源 MySQL
```

## 接口语义

点赞和取消点赞仍然使用原有接口：

- `POST /api/v1/videos/:id/like`
- `DELETE /api/v1/videos/:id/like`

建议保持 HTTP 200，并返回处理后的状态；前端可以继续做乐观计数。

如果 MQ 写入失败，接口必须返回错误，不能假装成功。

## 事件模型

### 点赞事件

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

### 取消点赞事件

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

## Redis Key

### 点赞用户集合

```text
like:video:{videoId}:users
```

Set，保存点赞该视频的用户 ID。

### 用户点赞集合

```text
like:user:{userId}:videos
```

Set，保存该用户点赞过的视频 ID。

### 视频实时统计

```text
video:stats:{videoId}
```

Hash，保存实时计数，例如：

- `like_count`
- `comment_count`
- `play_count`
- `hot_score`

### 脏视频集合

```text
like:dirty:videos
```

Set，保存需要批量刷回 MySQL 的视频 ID。

### 事件去重

```text
like:event:processed:{eventId}
```

String，记录事件是否已处理。

## 幂等规则

点赞：

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

重复消息不能重复加减计数。

## MySQL 刷库

第一阶段只同步点赞计数：

```text
video:stats:{videoId}.like_count -> video_stats.like_count
```

流程：

1. 从 `like:dirty:videos` 取一批 video ID。
2. 读取 Redis 点赞计数。
3. 批量更新 MySQL `video_stats.like_count`。
4. 清理脏标记。

完整点赞关系 diff 后续再补。

## 查询策略

- 点赞数优先读 Redis。
- 当前用户是否点赞优先查 Redis Set。
- `LikedVideos` 第一阶段可以继续保留 MySQL 查询。

## 降级

- MQ 写入失败，接口返回失败。
- Redis 不可用时，点赞链路不继续无保护地切回 MySQL。
- 刷库失败时，保留 dirty 标记，下次继续重试。

## 验证

- 点赞 / 取消点赞接口可用。
- MQ 能收到点赞请求事件。
- Worker 能更新 Redis key。
- 重复点赞 / 取消点赞不会重复改计数。
- 刷库任务能把 Redis 计数同步到 MySQL。
