# 项目进度

## 阶段 A

阶段 A 已完成点赞削峰的第一版实现：

- 新建了 `docs/protect-like-phase-a.md`。
- 点赞 / 取消点赞链路已经改成投递请求事件。
- Worker 已接入点赞事件消费和 Redis 实时状态处理。
- 批量刷回 MySQL 已落地：Worker 内置 30s 定时同步循环，通过 Lua 脚本原子读取脏视频状态 + 批量 UPSERT 关系行到 `likes` 表 + 清理已取消的点赞行。
- `Service.Status()` 查询路径已改为 Redis 优先（IsLiked + GetLikeCount），失败时透明回退 MySQL。
- 进度文档、数据库文档和相关设计文档已同步更新。

## 未完成

- `LikedVideos()` 和 `CountAuthorReceivedLikes()` 仍走 MySQL（Phase A 范围外）。
- 评论、关注还没有纳入同样的异步链路。
- 热点流 60 分钟滑动窗口还没有实现。

## 下一步

- 方案 A 代码收尾和验证。
- 按需求优先级继续扩展评论/关注异步链路、Outbox、热点流窗口。
