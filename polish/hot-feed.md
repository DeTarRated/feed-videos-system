## 60 分钟滑动窗口热点流设计

当前阶段 3 已经实现了简化版热点流：点赞、评论变化后重新计算视频热度分，并写入 Redis ZSET。这个方案实现简单，适合当前阶段验证 Redis 排行榜和热点流能力。

但它本质上更接近"累计热度榜"，不是严格意义上的"最近一段时间热点"。老视频如果累计互动很多，仍然可能长期占据热点流。

因此，后续将热点流升级为 **60 分钟滑动窗口热度模型**。

## 核心思路

建立一个 60 分钟滑动窗口，以 1 分钟为粒度记录热度增量。

每一分钟维护一个 Redis ZSET：

```text
hot:video:minute:{yyyyMMddHHmm}
```

例如：

```text
hot:video:minute:202607301430
hot:video:minute:202607301431
hot:video:minute:202607301432
```

每个 ZSET 中：

- member：视频 ID
- score：这一分钟内产生的热度增量

## 互动事件进入分钟桶

Worker 在处理点赞和评论事件时，在更新 Redis 状态（SADD、HINCRBY）之后，
**并行**拉出一条热度计算链路，直接将热度增量写入当前分钟桶。

```text
Worker 消费 LikeRequested:
  → SADD like:video:{id}:users {userId}        ← 现有逻辑
  → HINCRBY video:stats:{id} like_count 1      ← 现有逻辑
  → ZINCRBY hot:video:minute:{minuteKey} 5 {videoId}  ← 并行拉出

Worker 消费 CommentCreated:
  → HINCRBY video:stats:{id} comment_count 1   ← 现有逻辑
  → ZINCRBY hot:video:minute:{minuteKey} 3 {videoId}  ← 并行拉出
```

点赞和创建评论两条链路各自负责将对应的热度增量写入分钟桶。
取消点赞、删除评论不产生正向热度，不需要写入分钟桶（老热度会随时间自然滑出窗口）。

```text
互动权重（可配置）：
  点赞     +5
  评论     +3
  后续扩展：
  播放     +1
  完播     +4
  关注作者 +6
  分享     +8
  收藏     +5
```

## 60 分钟聚合榜

维护一个最近 60 分钟的聚合 ZSET：

```text
rank:video:hot:60m
```

这个 ZSET 表示当前时间点最近 60 分钟内的视频热度总和。

### 聚合规则（统一 1 分钟延迟）

为了避免事件延迟或乱序导致的丢失，聚合任务统一延迟 **1 分钟**执行：

```text
定时任务每分钟执行一次（例如每分钟的第 5 秒）：

  nowMinute = 当前分钟（例如 14:32）
  addKey    = hot:video:minute:{14:31}     ← 1 分钟前的桶（已不会有新事件写入）
  removeKey = hot:video:minute:{13:31}     ← 61 分钟前的桶（已滑出窗口）

  rank:video:hot:60m += 每个视频在 addKey 中的分数
  rank:video:hot:60m -= 每个视频在 removeKey 中的分数

  清理 removeKey（DEL 或依赖 TTL 自动过期）
```

如果 `removeKey` 已经过期（TTL 70 分钟），则跳过扣减步骤，窗口自然减小。

## 并行处理链路总结

```text
HTTP 点赞 → MQ → Worker ──┬── Redis 状态更新（like:video:{id}:users、video:stats:{id}）
                          │
                          └── 分钟桶写入（ZINCRBY hot:video:minute:{key} 5 {videoId}）
                                   │
                                   ▼
                          定时聚合（每分钟 1 次，延迟 1 分钟）
                                   │
                                   ▼
                          rank:video:hot:60m（聚合 ZSET）
                                   │
                                   ▼
                          热点流快照（feed:hot:snapshot:{id}，5min TTL）
```

两条路径在 Worker 内并行执行，互不阻塞。分钟桶写入失败不影响主链路。

## 低分视频清理

`rank:video:hot:60m` 中的视频随着老桶被扣减，分数逐渐下降。
清理策略（每 5 分钟执行一次）：

```text
// 清理分数 ≤ 0 的视频（已完全滑出窗口）
ZREMRANGEBYSCORE rank:video:hot:60m -inf 0

// 保留 Top 5000，超出部分移除（控制内存和 ZSET 大小）
ZCARD rank:video:hot:60m
if count > 5000:
    ZREMRANGEBYRANK rank:video:hot:60m 0 -{count-5000-1}
```

在扣减过程中某个视频分数可能降到 0，但该视频如果有持续互动，
分数会被后续分钟桶的增量再次推高，不会被误删。
理论上一分钟桶内的分数不会出现负值（只有正向事件写入），
所以扣减的结果最小为 0，不会出现负数。

## Worker 宕机处理

Worker 宕机期间的分钟桶为空，对应时间段的热度数据丢失。

当前策略：
- 分钟桶 TTL 设为 70 分钟（窗口 60 分钟 + 延迟 1 分钟 + 缓冲 9 分钟）。
- Worker 恢复后，直接从当前时间开始继续写入新桶。
- 宕机期间的分钟桶不存在，聚合时 `addKey` 为空（跳过），
  `removeKey` 可能已过期（跳过），窗口自然收缩。
- 宕机 5 分钟内恢复：只有 5 分钟的热度缺失，对整个榜单影响可控。
- 宕机超过 60 分钟：窗口内全部桶为空，榜单完全冷启动，
  新热点会在 60 分钟内重新建立。

后续改进方向：
- Worker 恢复后回放 MQ 中积压的事件（消息中的 `occurredAt` 时间戳
  可以定位到正确的分钟桶，补回缺失的热度数据）。
- 如果 MQ 积压超过 60 分钟，只回放最近 60 分钟的事件即可。

## 热点流快照

热点流接口继续使用已设计的快照方案：

```text
GET /api/v1/feed/hot?limit=20&snapshotId=...&lastVideoId=...
```

区别是数据源从 `rank:video:hot` 切到 `rank:video:hot:60m`。
快照 TTL 保持 **5 分钟**。

## 与当前架构的对接

当前 Worker 已具备以下能力：

- 消费点赞/评论事件（`handleLikeEvent`、`handleCommentEvent`）
- 定时任务（`syncLoop`，30s 间隔）
- 操作 Redis ZSET（`HotVideoRank.SetScore`）

改动点：

| 模块 | 改动 |
|------|------|
| Worker `handleLikeEvent` | 新增 `ZINCRBY hot:video:minute:{key} 5 {videoId}` |
| Worker `handleCommentEvent` | 新增 `ZINCRBY hot:video:minute:{key} 3 {videoId}` |
| Worker 新增聚合任务 | 每 60s 执行一次窗口滑动（addKey +1、removeKey -1、清理） |
| Worker 新增清理任务 | 每 300s 执行一次 `ZREMRANGEBYSCORE` + `ZREMRANGEBYRANK` |
| 热点流 Handler | 数据源从 `rank:video:hot` 切到 `rank:video:hot:60m` |
