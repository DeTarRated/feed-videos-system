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

## 批量刷库 Lua 脚本

### 目的

定时将 Redis 中的点赞关系同步回 MySQL `likes` 表。
Worker 每 30 秒从脏视频集合取样，通过 Lua 脚本原子地读取并清理单个视频的脏状态，再批量写入 MySQL。

### 脚本

```lua
local users_key = KEYS[1]   -- like:video:{videoId}:users
local stats_key = KEYS[2]   -- video:stats:{videoId}
local dirty_key = KEYS[3]   -- like:dirty:videos
local video_id  = ARGV[1]

local users = redis.call('SMEMBERS', users_key)
local count = redis.call('HGET', stats_key, 'like_count')
redis.call('SREM', dirty_key, video_id)

local result = { count or '0' }
for i, u in ipairs(users) do
    result[#result + 1] = u
end
return result
```

### 调用方式

Worker 中 `SyncDirtyVideo` 方法传入 `videoID`，自动构建三个 key：

```text
KEYS = [
    "like:video:{videoId}:users",
    "video:stats:{videoId}",
    "like:dirty:videos"
]
ARGV = [ "{videoId}" ]
```

返回值是一个数组，第一个元素为 `like_count`，后续元素为点赞用户 ID。

### 原子性说明

把 `SMEMBERS` + `HGET` + `SREM` 打包为一个 Lua 脚本，消除分步操作之间的竞态窗口：

```text
分步操作（有竞态）：
  SMEMBERS → [1,2,3]
  --- 此时 Worker 写入新用户 4，SADD dirty ---
  SREM dirty            ← 清掉了刚标记的脏！

Lua 脚本（无竞态）：
  SMEMBERS + HGET + SREM ← 同一原子事务内执行
  --- 之后 Worker 写入新用户 4，SADD dirty ---
  dirty 中重新包含视频 ← 下一轮同步会处理
```

### 刷库流程

```text
1. SRANDMEMBER like:dirty:videos 100    -- 随机取最多 100 个脏视频
2. 对每个视频，执行上述 Lua 脚本          -- 原子读 + 清脏
3. MySQL 批量 UPSERT 用户-视频关系行     -- ON CONFLICT DO UPDATE
4. MySQL 清理不在 Redis Set 中的行       -- soft delete 已取消点赞
5. 任一步失败 → SADD like:dirty:videos    -- 重新标记，下轮重试
```

## 防缓存击穿（singleflight）

### 问题

`Service.Status()` 查询链路为 Redis 优先 → MySQL 回退。
当 Redis 中对应 key 集体过期或 Redis 短暂不可用时，
所有并发请求同时穿透到 MySQL，造成缓存击穿。

### 方案

在 MySQL 回退路径上使用 `golang.org/x/sync/singleflight`，
同一 `(userID, videoID)` 组合的并发请求只执行一次 MySQL 查询，其余 goroutine 等待共享结果。

```text
并发请求 × N（同一 video + user，Redis 未命中）
  │
  ├─ g.Do("db:like:status:{userID}:{videoID}", fn)
  │    ├─ 第一个到达：执行 statusFromDB()，查询 MySQL
  │    └─ 其余 N-1 个：等待，共享第一个的结果
  │
  └─ 所有请求拿到同一个 *LikeStatus
```

### 作用范围

- 仅包裹 MySQL 回退路径（`statusFromDB`）。
- Redis 命中时不做 singleflight（无必要，Redis 已是 sub-ms 响应）。
- `singleflight.Group` 零值可用，无需构造参数。
