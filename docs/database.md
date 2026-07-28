# 数据库设计

## 当前阶段范围

当前 MVP 已实现的核心表：

- `users`
- `videos`

当前阶段的用户主页统计和作者作品列表均基于上述两张表实时查询，不依赖额外统计表。

## users

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 用户 ID |
| `username` | varchar(64) unique | 用户名 |
| `password` | varchar(255) | 密码哈希 |
| `nickname` | varchar(64) | 昵称 |
| `avatar_url` | varchar(255) | 头像地址 |
| `bio` | varchar(255) | 个人简介 |
| `created_at` | datetime(3) | 创建时间 |
| `updated_at` | datetime(3) | 更新时间 |

### 索引

- `uk_users_username`：用户名唯一索引。

## videos

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 视频 ID |
| `author_id` | bigint unsigned | 作者 ID |
| `title` | varchar(128) | 视频标题 |
| `description` | varchar(500) | 视频描述 |
| `video_url` | varchar(255) | 视频文件地址 |
| `cover_url` | varchar(255) | 封面地址 |
| `published_at` | datetime(3) | 发布时间 |
| `created_at` | datetime(3) | 创建时间 |
| `updated_at` | datetime(3) | 更新时间 |

### 索引

- `idx_videos_author_created(author_id, created_at, id)`：作者作品列表查询。
- `idx_videos_published(published_at, id)`：最新流查询。

### 关系

- `videos.author_id` 关联 `users.id`。

## 当前阶段说明

- 用户主页统计中的作品数来自 `videos` 按 `author_id` 计数。
- 获赞数、粉丝数、关注数在当前阶段先作为预留字段返回 `0`，后续引入互动表后再补。
