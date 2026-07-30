## 60 分钟滑动窗口热点流设计

当前阶段 3 已经实现了简化版热点流：点赞、评论变化后重新计算视频热度分，并写入 Redis ZSET。这个方案实现简单，适合当前阶段验证 Redis 排行榜和热点流能力。

但它本质上更接近“累计热度榜”，不是严格意义上的“最近一段时间热点”。老视频如果累计互动很多，仍然可能长期占据热点流。

因此，后续阶段 4 计划将热点流升级为 **60 分钟滑动窗口热度模型**。

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

互动事件进入当前分钟桶：

```text
点赞：ZINCRBY hot:video:minute:{minuteKey} 5 {videoId}
评论：ZINCRBY hot:video:minute:{minuteKey} 3 {videoId}
```

后续也可以扩展：

```text
播放：+1
完播：+4
关注作者：+6
分享：+8
收藏：+5
```

## 60 分钟聚合榜

维护一个最近 60 分钟的聚合 ZSET：

```text
rank:video:hot:60m
```

这个 ZSET 表示当前时间点最近 60 分钟内的视频热度总和。

每分钟窗口向前滑动一次：

```text
rank:video:hot:60m += 最近 1 分钟桶
rank:video:hot:60m -= 60 分钟前的分钟桶
```

也就是：

1. 找到当前刚结束的 1 分钟桶。
2. 把这个桶里的每个视频分数加到 `rank:video:hot:60m`。
3. 找到 60 分钟前的桶。
4. 把这个桶里的每个视频分数从 `rank:video:hot:60m` 中减掉。
5. 删除或设置过期 60 分钟前的桶。

这样可以得到真正的“最近 60 分钟热门视频”。

## 为什么放到阶段 4 实现

60 分钟滑动窗口更适合配合异步事件和 Worker 实现。

阶段 4 会引入 RabbitMQ 和 Worker，点赞、评论、关注等事件可以先写入消息队列，再由 Worker 统一消费并写入 Redis 分钟桶。

这样可以避免把复杂的热度计算逻辑塞进 HTTP 请求里。

当前阶段 3 的职责是：

- 接入 Redis。
- 实现视频详情缓存。
- 实现游标分页。
- 实现简化版热点流。
- 实现限流。

阶段 4 再处理：

- 互动事件异步化。
- 热度增量写入分钟桶。
- 每分钟聚合滑动窗口。
- Worker 失败重试。
- MQ 不可用时的降级策略。

## 和当前方案对比

| 方案 | 计算方式 | 优点 | 缺点 | 适合阶段 |
| --- | --- | --- | --- | --- |
| 当前简化版热点流 | 直接按点赞数、评论数和发布时间重算热度分 | 简单，容易验证 | 更像累计榜，老视频可能长期占榜 | 阶段 3 |
| 60 分钟滑动窗口 | 聚合最近 60 个 1 分钟热度桶 | 更符合近期热点，能反映短时间热度变化 | 需要 Worker、定时任务和事件流 | 阶段 4 |

## 推荐阶段 4 实现方案

### 1. 事件进入 MQ

点赞、取消点赞、评论、删除评论、关注等行为完成数据库操作后，发送事件到 RabbitMQ。

事件示例：

```json
{
  "type": "video_liked",
  "videoId": 1,
  "userId": 2,
  "scoreDelta": 5,
  "occurredAt": "2026-07-30T14:30:20Z"
}
```

### 2. Worker 写入分钟桶

Worker 消费事件后，根据 `occurredAt` 计算分钟桶 key：

```text
hot:video:minute:202607301430
```

然后执行：

```text
ZINCRBY hot:video:minute:202607301430 {scoreDelta} {videoId}
```

分钟桶 TTL 可以设置为 70 分钟，略大于窗口长度，方便延迟事件处理。

### 3. 定时聚合窗口

Worker 每分钟执行一次窗口滑动：

```text
addKey = hot:video:minute:{当前刚结束分钟}
removeKey = hot:video:minute:{60分钟前分钟}
rankKey = rank:video:hot:60m
```

对 `addKey` 中的视频分数执行正向增量：

```text
ZINCRBY rank:video:hot:60m +score videoId
```

对 `removeKey` 中的视频分数执行反向扣减：

```text
ZINCRBY rank:video:hot:60m -score videoId
```

如果某个视频聚合分数小于等于 0，可以从 `rank:video:hot:60m` 中移除。

### 4. 热点流快照

热点流接口继续使用阶段 3 已设计的快照方案：

```text
GET /api/v1/feed/hot?limit=20&snapshotId=...&lastVideoId=...
```

区别是：

- 阶段 3 快照来自 `rank:video:hot`
- 阶段 4 快照来自 `rank:video:hot:60m`

快照 TTL 保持 **5 分钟**。

## 面试讲解要点

可以这样解释演进路线：

> 阶段 3 我先用 Redis ZSET 实现了简化版热点流，点赞和评论后同步更新热度分，先保证热点流功能可用。阶段 4 引入 RabbitMQ 和 Worker 后，会把热点计算升级为 60 分钟滑动窗口：每个互动事件先进入 MQ，Worker 按 1 分钟粒度写入热度桶，再定时维护最近 60 分钟聚合榜。热点流接口基于聚合榜生成 5 分钟快照，保证用户连续刷视频时顺序稳定。

这套设计能同时解决：

- 热点计算异步化，降低接口耗时。
- 最近 60 分钟窗口更准确表达“近期热门”。
- 5 分钟快照保证用户浏览稳定。
- Worker 和 MQ 方便做重试、补偿和降级。
