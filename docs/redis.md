# Redis 设计

## 连接配置

```text
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0
VIDEO_DETAIL_CACHE_TTL=10m
RATE_LIMIT_WINDOW=1m
```

启动时会连接 Redis 并执行 `PING` 检查。

## Key 设计

| 场景 | Key | 类型 | TTL | 说明 |
| --- | --- | --- | --- | --- |
| 视频详情缓存 | `video:detail:{videoId}` | String JSON | `VIDEO_DETAIL_CACHE_TTL`，默认 10 分钟 | 保存视频详情接口响应数据 |
| 热榜 | `rank:video:hot` | ZSET | 无固定 TTL | member 为视频 ID，score 为热度分 |
| 热点流快照 | `feed:hot:snapshot:{snapshotId}` | String JSON | 5 分钟 | 保存热点流分页快照 |
| 限流 | `rate_limit:{ruleName}:{identity}` | String Counter | 窗口时间 | 保存当前窗口内请求次数 |
| 点赞用户集合 | `like:video:{videoId}:users` | Set | 30 天 | 保存点赞某个视频的用户 ID |
| 用户点赞集合 | `like:user:{userId}:videos` | Set | 30 天 | 保存某个用户点赞过的视频 ID |
| 视频实时统计 | `video:stats:{videoId}` | Hash | 30 天 | 保存视频实时计数 |
| 脏视频集合 | `like:dirty:videos` | Set | 30 天 | 保存需要刷回 MySQL 的视频 ID |
| 点赞事件去重 | `like:event:processed:{eventId}` | String | 1 天 | 记录点赞事件是否已处理 |

## 视频详情缓存

### Key

```text
video:detail:{videoId}
```

### 数据类型

String。

### 内容

视频详情接口响应 JSON。

### 读写流程

1. 先读 Redis。
2. 命中则直接返回。
3. 未命中则回源 MySQL。
4. 查询成功后写回 Redis。

### 失效策略

点赞、取消点赞、评论、新增删除评论后删除对应视频详情缓存。

## 热榜

### Key

```text
rank:video:hot
```

### 数据类型

ZSET。

- member：视频 ID。
- score：热度分。

## 热点流快照

### Key

```text
feed:hot:snapshot:{snapshotId}
```

### 数据类型

String。

### 内容

热点流分页时保存的视频 ID 列表和创建时间。

### 分页参数

第一页请求：

```text
GET /api/v1/feed/hot?limit=20
```

下一页请求：

```text
GET /api/v1/feed/hot?limit=20&snapshotId={snapshotId}&lastVideoId={lastVideoId}
```

## 固定窗口限流

### Key

```text
rate_limit:{ruleName}:{identity}
```

### 数据类型

String。

### 实现方式

1. `INCR`。
2. 首次请求时设置 `EXPIRE`。
3. 超过限制返回 HTTP 429。

## 点赞实时状态

### 视频点赞用户集合

```text
like:video:{videoId}:users
```

Set，保存点赞该视频的用户 ID。

### 用户点赞视频集合

```text
like:user:{userId}:videos
```

Set，保存某个用户点赞过的视频 ID。

### 视频实时统计

```text
video:stats:{videoId}
```

Hash。

常见字段：

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

Set，保存需要批量刷回 MySQL 的视频 ID。

### 点赞事件去重

```text
like:event:processed:{eventId}
```

String，记录某个点赞事件是否已处理。

## 点赞幂等

点赞：

```text
SADD like:video:{videoId}:users {userId}
SADD like:user:{userId}:videos {videoId}
HINCRBY video:stats:{videoId} like_count 1
SADD like:dirty:videos {videoId}
```

取消点赞：

```text
SREM like:video:{videoId}:users {userId}
SREM like:user:{userId}:videos {videoId}
HINCRBY video:stats:{videoId} like_count -1
SADD like:dirty:videos {videoId}
```

实际处理必须依赖 `SADD` / `SREM` 的返回值，避免重复消息导致重复加减。
