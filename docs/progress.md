# 项目进度

## 阶段 A

阶段 A 已完成点赞削峰的第一版实现：

- 新建了 `docs/protect-like-phase-a.md`。
- 点赞 / 取消点赞链路已经改成投递请求事件。
- Worker 已接入点赞事件消费和 Redis 实时状态处理。
- 批量刷回 MySQL 已落地：Worker 内置 30s 定时同步循环，通过 Lua 脚本原子读取脏视频状态 + 批量 UPSERT 关系行到 `likes` 表 + 清理已取消的点赞行。
- `Service.Status()` 查询路径已改为 Redis 优先（IsLiked + GetLikeCount），失败时透明回退 MySQL。
- 进度文档、数据库文档和相关设计文档已同步更新。

## 已实现（评论异步链路）

- 创建评论保持同步写 MySQL，副作用（计数、热度、缓存失效）通过 MQ 事件下沉到 Worker。
- 删除评论改为全异步：HTTP 校验 → 发布事件 → Worker 写 MySQL + 更新 Redis。
- 评论列表首页加 `comment:first_page:{videoId}` 缓存（30s TTL）。
- 评论详情加 `comment:detail:{commentId}` 缓存（5min TTL），支持异步删除路径。
- `video:stats:{videoId}` Hash 新增 `comment_count` 字段，Worker 通过 `HINCRBY` 维护。

## 未完成

- `LikedVideos()` 和 `CountAuthorReceivedLikes()` 仍走 MySQL（Phase A 范围外）。
- 关注还没有纳入异步链路。
- 热点流 60 分钟滑动窗口还没有实现。

## 下一步

- 方案 A 代码收尾和验证。
- 按需求优先级继续扩展关注异步链路、Outbox、热点流窗口。
