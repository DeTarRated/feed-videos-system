# 阶段 3：Feed 与缓存设计

> 本文档用于详细拆解阶段 3 要实现的 Redis 缓存、Feed 游标分页、热榜和接口限流能力。阶段总览见 `plan.md`，当前进度见 `docs/progress.md`。

## 目标

- 引入 Redis，让项目具备可讲解的缓存、排行榜和限流能力。
- 优先优化视频详情和 Feed 查询这类读多写少场景。
- 将 Feed 从简单列表升级为稳定游标分页，避免 offset 分页在数据变化时重复或漏数据。
- 用 Redis ZSET 实现热榜，为后续异步事件和 Worker 做铺垫。
- 保持阶段 3 范围可验证、可提交，不提前引入 RabbitMQ 或复杂推荐算法。

## 功能范围

### 1. Redis 基础设施

- [x] `docker-compose.yml` 增加 Redis 服务。
- [x] `.env.example` 增加 Redis 配置项。
- [x] 后端配置结构增加 Redis 地址、密码、DB、连接超时和默认 TTL。
- [x] 应用启动时初始化 Redis 客户端。
- [x] 应用退出时关闭 Redis 客户端。
- [x] Redis 不可用时明确行为：阶段 3 推荐先让启动失败，避免缓存和限流状态不确定。

推荐配置项：

| 配置项 | 示例 | 说明 |
| --- | --- | --- |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `REDIS_PASSWORD` | 空 | 本地开发可为空 |
| `REDIS_DB` | `0` | Redis DB 编号 |
| `VIDEO_DETAIL_CACHE_TTL` | `10m` | 视频详情缓存时间 |
| `RATE_LIMIT_WINDOW` | `1m` | 限流窗口 |

## Redis Key 设计

| 场景 | Key | 类型 | TTL | 说明 |
| --- | --- | --- | --- | --- |
| 视频详情缓存 | `video:detail:{videoId}` | String JSON | 10 分钟 | 缓存视频详情接口响应数据 |
| 热榜 | `rank:video:hot` | ZSET | 不设置或定期重建 | member 为视频 ID，score 为热度分 |
| 限流 | `rate_limit:{route}:{identity}` | String Counter | 窗口时间 | identity 优先使用用户 ID，匿名使用 IP |
| 可选 Token 状态 | `auth:token:{tokenId}` | String | Token 有效期 | 阶段 3 可选，不作为第一优先级 |

命名原则：

- Key 使用业务域前缀，例如 `video:`、`rank:`、`rate_limit:`。
- Key 中只放稳定 ID，不放昵称、标题等可变文本。
- 缓存值优先存接口需要的结构，减少反复组装数据。

## 视频详情缓存

### 实现状态

- [x] `GET /api/v1/videos/:id` 使用 cache-aside 模式读取 Redis。
- [x] 缓存 Key 使用 `video:detail:{videoId}`。
- [x] 缓存内容为视频详情接口最终响应结构。
- [x] Redis 读取或写入失败时记录日志并降级查询 MySQL。
- [x] 点赞、取消点赞、发表评论和删除评论后删除对应视频详情缓存。

### 查询流程

1. 请求 `GET /api/v1/videos/:id`。
2. 根据视频 ID 读取 `video:detail:{videoId}`。
3. Redis 命中：反序列化后直接返回。
4. Redis 未命中：查询 MySQL，组装详情响应，写入 Redis，再返回。
5. Redis 读取或写入失败：记录日志，降级查询 MySQL。

### 缓存内容

推荐缓存视频详情接口最终响应中的核心字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 视频 ID |
| `authorId` | 作者 ID |
| `title` | 标题 |
| `description` | 描述 |
| `videoUrl` | 视频地址 |
| `coverUrl` | 封面地址 |
| `likeCount` | 点赞数 |
| `commentCount` | 评论数 |
| `createdAt` | 发布时间 |

### 失效策略

- 发布视频：新视频没有旧缓存，不需要删除。
- 修改视频：删除 `video:detail:{videoId}`。
- 删除视频：删除 `video:detail:{videoId}`。
- 点赞、取消点赞、评论、删除评论：如果详情响应里包含点赞数或评论数，需要删除对应详情缓存，避免展示旧统计。

当前项目还没有视频修改和删除接口，阶段 3 可以先处理点赞和评论导致的统计变化。

## 最新流游标分页

### 实现状态

- [x] `GET /api/v1/videos` 支持 `limit` 和 `cursor` 参数。
- [x] 使用 `published_at DESC, id DESC` 保证排序稳定。
- [x] Cursor 使用 base64url 编码，内部包含 `publishedAt` 和 `id`。
- [x] 响应返回 `items`、`nextCursor` 和 `hasMore`。
- [x] 前端 Feed 页面支持加载更多。

### 目标

当前最新 Feed 如果只使用 `limit` 或后续改成 offset，在有新视频发布时容易出现重复或漏数据。阶段 3 推荐改成基于游标的稳定分页。

### 排序规则

```text
ORDER BY published_at DESC, id DESC
```

`published_at` 相同时用 `id` 作为第二排序字段，保证顺序稳定。

### Cursor 内容

推荐 cursor 编码两个字段：

```json
{
  "createdAt": "2026-07-30T12:00:00Z",
  "id": 123
}
```

接口可以用 base64url 编码，避免前端理解内部结构。

### 请求参数

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `limit` | 否 | 每页数量，默认 20，最大 50 |
| `cursor` | 否 | 上一页返回的 `nextCursor` |

### 响应结构

```json
{
  "items": [
    {
      "id": 1,
      "authorId": 2,
      "title": "示例视频",
      "description": "视频描述",
      "videoUrl": "/media/videos/demo.mp4",
      "coverUrl": "/media/covers/demo.jpg",
      "createdAt": "2026-07-30T12:00:00Z"
    }
  ],
  "nextCursor": "base64urlCursor",
  "hasMore": true
}
```

### 查询条件

第一页不带 cursor：

```sql
SELECT * FROM videos
ORDER BY published_at DESC, id DESC
LIMIT ?;
```

后续页带 cursor：

```sql
SELECT * FROM videos
WHERE published_at < ? OR (published_at = ? AND id < ?)
ORDER BY published_at DESC, id DESC
LIMIT ?;
```

实际查询建议多取一条：`limit + 1`。如果多出来一条，说明还有下一页，用最后一条可见数据生成 `nextCursor`。

## 热榜

### 实现状态

- [x] 使用 Redis ZSET `rank:video:hot` 存储视频热度分。
- [x] 发布视频后写入初始热度分。
- [x] 点赞、取消点赞、发表评论和删除评论后同步重算热度分。
- [x] 新增 `GET /api/v1/feed/hot` 热榜接口。
- [x] 前端首页右侧展示热榜视频。

### 数据结构

使用 Redis ZSET：

```text
rank:video:hot
```

- member：视频 ID。
- score：热度分数。

### 初版分数公式

阶段 3 可以先使用简单、可解释的同步公式：

```text
score = likeCount * 5 + commentCount * 3 + publishScore
```

`publishScore` 可以用发布时间转换出的时间分，保证新内容有一定曝光机会。后续阶段 4 引入 Worker 后，再把热度更新迁移到异步事件中。

### 更新时机

- 发布视频：新增视频到热榜。
- 点赞：增加分数或重新计算。
- 取消点赞：减少分数或重新计算。
- 发布评论：增加分数或重新计算。
- 删除评论：减少分数或重新计算。

阶段 3 推荐优先使用“重新计算后写入 ZSET”，逻辑更直观，避免增量更新和数据库统计不一致。

### 查询接口

建议新增：

| 功能 | 方法 | URL | 鉴权 | 说明 |
| --- | --- | --- | --- | --- |
| 热榜视频 | `GET` | `/api/v1/feed/hot?limit=20` | 无 | 按 Redis ZSET 分数倒序返回视频列表 |

如果 ZSET 为空，可以降级查询最新视频，避免前端空白。

## 接口限流

### 实现状态

- [x] 基于 Redis 固定窗口实现限流中间件。
- [x] 登录和注册接口按 IP 限流。
- [x] 发布、点赞、评论和关注写接口按登录用户 ID 限流。
- [x] Redis 运行期读写失败时记录日志并放行业务请求。
- [x] 超过阈值时返回 HTTP 429 和 `too many requests`。
- [~] 等待 Apifox 手动验证限流规则。

### 范围

阶段 3 先保护高频写接口：

| 接口类型 | 建议限制 |
| --- | --- |
| 登录 | 每 IP 每分钟 10 次 |
| 注册 | 每 IP 每分钟 5 次 |
| 发布视频 | 每用户每分钟 5 次 |
| 点赞/取消点赞 | 每用户每分钟 60 次 |
| 评论 | 每用户每分钟 20 次 |
| 关注/取关 | 每用户每分钟 30 次 |

### 实现方式

使用 Redis 固定窗口计数：

1. 根据路由和身份生成 key：`rate_limit:{route}:{identity}`。
2. `INCR` 计数。
3. 第一次创建 key 时设置过期时间。
4. 超过限制返回 `429 Too Many Requests`。

响应示例：

```json
{
  "code": 429,
  "message": "too many requests",
  "data": null
}
```

固定窗口实现简单，适合当前阶段；滑动窗口和令牌桶可以作为面试扩展点。

## 后端实现顺序

1. 增加 Redis 配置和客户端。
2. 增加缓存 helper 或 `cache` infrastructure 包。
3. 在视频详情 service 或 repository 上层接入缓存。
4. 扩展视频列表接口为游标分页。
5. 增加热榜 service 和接口。
6. 增加限流 middleware，并挂到指定路由。
7. 补充 Go test，覆盖 cursor 编解码、限流判断和缓存降级逻辑。

## 前端实现顺序

1. 调整视频列表接口类型，支持 `nextCursor` 和 `hasMore`。
2. Feed 页面增加“加载更多”或滚动触发下一页。
3. 增加热榜入口或右侧热榜区域。
4. 对限流错误展示明确提示。

## Apifox 验证清单

### Redis 基础设施

- 启动 Redis 和后端。
- 调用 `/healthz` 确认服务可用。
- 关闭 Redis 后确认后端行为符合设计：启动失败或明确降级。

### 视频详情缓存

- 请求方法：`GET`
- URL：`{{baseUrl}}/api/v1/videos/{{videoId}}`
- 验证点：
  - 第一次请求从 MySQL 返回正常详情。
  - 第二次请求仍返回相同结构。
  - 点赞或评论后再次请求，统计字段不应长期停留在旧值。

### 最新流游标分页

- 请求方法：`GET`
- URL：`{{baseUrl}}/api/v1/videos?limit=5`
- 下一页 URL：`{{baseUrl}}/api/v1/videos?limit=5&cursor={{nextCursor}}`
- 验证点：
  - 返回 `items`、`nextCursor`、`hasMore`。
  - 下一页不重复上一页最后一个视频。
  - 数据不足时 `hasMore` 为 `false`。

### 热榜

- 请求方法：`GET`
- URL：`{{baseUrl}}/api/v1/feed/hot?limit=10`
- 验证点：
  - 返回视频列表。
  - 点赞和评论后，视频热度排序有变化或分数被更新。

### 限流

- 选择登录、评论或点赞接口连续快速请求。
- 验证超过阈值后返回：
  - HTTP 状态码：`429`
  - 响应 `message`：`too many requests`

## 面试讲解要点

- Redis 在本项目中不是“为了用而用”，而是分别解决视频详情读多写少、热榜排序和高频接口限流问题。
- 视频详情缓存采用 cache-aside 模式：业务先读缓存，未命中再读数据库并回填。
- 统计字段变化时删除详情缓存，比直接修改缓存更容易保持一致。
- Feed 使用游标分页而不是 offset，是为了避免新数据插入导致翻页重复或漏数据。
- 热榜用 ZSET 是因为它天然支持按分数排序和 Top N 查询。
- 固定窗口限流实现简单，适合 MVP；令牌桶或滑动窗口可以作为后续优化。
- 阶段 4 引入 RabbitMQ 后，可以把热度更新从同步逻辑迁移到异步 Worker。
