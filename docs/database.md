# 数据库设计

## 当前表

- `users`：用户账号和资料。
- `videos`：视频基础信息。
- `likes`：用户与视频的点赞关系，支持软删除。
- `comments`：视频评论，支持软删除。
- `follows`：用户关注关系，支持软删除。

## users

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 用户 ID |
| `username` | varchar(64) unique | 用户名 |
| `password` | varchar(255) | 密码哈希 |
| `nickname` | varchar(64) | 昵称 |
| `avatar_url` | varchar(255) | 头像地址 |
| `bio` | varchar(255) | 个人简介 |
| `created_at` / `updated_at` | datetime(3) | 时间字段 |

## videos

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 视频 ID |
| `author_id` | bigint unsigned | 作者 ID，关联 `users.id` |
| `title` | varchar(128) | 标题 |
| `description` | varchar(500) | 描述 |
| `video_url` | varchar(255) | 视频地址 |
| `cover_url` | varchar(255) | 封面地址 |
| `published_at` | datetime(3) | 发布时间 |

## likes

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 记录 ID |
| `user_id` | bigint unsigned | 点赞用户 |
| `video_id` | bigint unsigned | 被点赞视频 |
| `created_at` | datetime(3) | 首次点赞时间 |
| `updated_at` | datetime(3) | 最近状态变更时间 |
| `deleted_at` | datetime(3) nullable | 取消点赞时间，非空表示已取消 |

索引：

- `uk_likes_user_video(user_id, video_id)`：保证同一用户对同一视频只有一条关系记录。
- `idx_likes_video_deleted(video_id, deleted_at)`：统计视频有效点赞数。
- `idx_likes_user_deleted(user_id, deleted_at)`：查询我的点赞视频。

点赞采用软删除式状态：点赞恢复记录，取消点赞写入 `deleted_at`，重复操作保持幂等。

## comments

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 评论 ID |
| `user_id` | bigint unsigned | 评论作者 |
| `video_id` | bigint unsigned | 所属视频 |
| `content` | varchar(500) | 评论内容 |
| `created_at` | datetime(3) | 创建时间 |
| `updated_at` | datetime(3) | 更新时间 |
| `deleted_at` | datetime(3) nullable | 删除时间，非空表示已删除 |

索引：

- `idx_comments_video_created(video_id, created_at, id)`：按视频查询评论并排序。
- `idx_comments_user_deleted(user_id, deleted_at)`：查询用户评论和删除状态。

评论删除使用软删除，并且后端要求 `comment_id` 和当前用户 ID 同时匹配，保证只能删除自己的评论。

## follows

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned PK | 关注关系 ID |
| `follower_id` | bigint unsigned | 关注发起用户 |
| `following_id` | bigint unsigned | 被关注用户 |
| `created_at` | datetime(3) | 首次关注时间 |
| `updated_at` | datetime(3) | 最近状态变更时间 |
| `deleted_at` | datetime(3) nullable | 取消关注时间，非空表示已取消 |

索引：

- `uk_follows_follower_following(follower_id, following_id)`：保证同一用户对同一目标只有一条关系记录。
- `idx_follows_follower_deleted(follower_id, deleted_at)`：查询关注列表和关注数。
- `idx_follows_following_deleted(following_id, deleted_at)`：查询粉丝列表和粉丝数。

关注采用软删除式状态：关注恢复记录，取消关注写入 `deleted_at`，重复操作保持幂等。数据库和领域服务都禁止用户关注自己。

## 迁移文件

- `001_init_users_videos.sql`
- `002_init_social_likes.sql`
- `003_init_social_comments.sql`
- `004_init_social_follows.sql`

当前 Docker Compose 只会自动执行首次初始化时挂载的迁移文件；已有 MySQL volume 需要手动执行新增迁移。
