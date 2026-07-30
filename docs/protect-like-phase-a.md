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

### 目标

将 Redis 中的点赞关系完整对账到 MySQL `likes` 表，而非只同步计数。
因为 `likes` 使用软删除模式，计数由 `COUNT(*) WHERE deleted_at IS NULL` 派生，
所以必须同步用户-视频关系行。

### 刷库链路

```text
Worker 定时器（30s）
  │
  ├─ 1. SRANDMEMBER like:dirty:videos 100
  │     取最多 100 个随机脏视频 ID
  │
  ├─ 2. 对每个视频，执行 Lua 脚本（原子读 + 清脏）：
  │      SMEMBERS like:video:{videoId}:users
  │      HGET video:stats:{videoId} like_count
  │      SREM like:dirty:videos {videoId}
  │
  ├─ 3. 批量 UPSERT 到 MySQL likes 表：
  │      INSERT INTO likes (user_id, video_id, created_at, updated_at)
  │      VALUES (u1, v1), (u2, v1), ...
  │      ON CONFLICT (user_id, video_id)
  │      DO UPDATE SET deleted_at = NULL, updated_at = now
  │
  ├─ 4. 清理 MySQL 中已不在 Redis Set 的记录（软删除）：
  │      UPDATE likes SET deleted_at = NOW()
  │      WHERE video_id = ? AND deleted_at IS NULL
  │        AND user_id NOT IN (Redis Set 中的用户 ID)
  │
  └─ 5. 失败处理：
         ─ MySQL 写入失败 → SADD like:dirty:videos {videoId}（重新标记）
         ─ DeleteLikesNotInSet 失败 → 非致命，下一轮重试
```

### 为什么需要 Lua 脚本

步骤 2 如果分成两步（先 SMEMBERS 再 SREM），中间存在竞态窗口：

```text
Sync:  SMEMBERS → [1,2,3]
Worker: SADD user 4, SADD dirty     ← 新点赞重新标记脏
Sync:  SREM dirty                   ← 清掉了刚标记的脏！
```

用 Lua 把 SMEMBERS + SREM 打包成原子操作，如果新点赞在 SREM 之后到达，
Worker 会再执行 `SADD dirty`，视频留到下一轮同步。不会丢失。

### 一致性保证

- **新增点赞**：Worker 写入 Redis 后 `SADD dirty`，保证下次或本轮同步覆盖。
- **取消点赞**：Worker 从 Redis Set 移除用户后 `SADD dirty`，同步时步骤 4 的
  `DeleteLikesNotInSet` 会把 MySQL 中的行软删除。
- **MySQL 写入失败**：重新 `SADD dirty`，下轮重试。
- **Lua 执行后 MySQL 写入前宕机**：dirty 标记已清除，但 Redis 数据未持久化。
  后续可通过全量对账任务弥补（Phase B / Outbox 之后补充）。

## 查询策略

- **`Status()`（`GET /videos/:id/like-status`）已切换为 Redis 优先**：
  同时读取 `IsLiked`（SISMEMBER）和 `GetLikeCount`（HGET），
  任一步失败则透明回退 MySQL。未登录用户跳过 `IsLiked` 调用。
- `LikedVideos()` 和 `CountAuthorReceivedLikes()` 仍走 MySQL（Phase A 范围外）。

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
