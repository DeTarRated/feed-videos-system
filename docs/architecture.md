# 架构设计

## 总体架构

系统采用前后端分离架构，后端提供 HTTP API，前端通过浏览器访问后端接口完成短视频浏览、发布、互动和消息通知等功能。

MVP 阶段先保证核心链路简单可运行：

```text
Browser
  |
  | HTTP / JSON / multipart
  v
Vue 3 Frontend
  |
  | REST API
  v
Go API Service
  |
  | GORM
  v
MySQL

Go API Service
  |
  | local filesystem
  v
Local Media Storage
```

后续增强阶段引入 Redis、RabbitMQ 和 Worker：

```text
Browser
  |
  | HTTP / SSE
  v
Vue 3 Frontend
  |
  v
Go API Service
  |       |          |
  |       |          +--> Redis: cache, ranking, token, rate limit, upload session
  |       |
  |       +------------> RabbitMQ: business events
  |
  +--------------------> MySQL: core data

RabbitMQ
  |
  v
Go Worker
  |
  +--> MySQL: async write and statistics
  +--> Redis: hot score, feed timeline, counters
  +--> Notification stream
```

核心原则是同步链路只做必要校验和关键写入，非关键统计、热度计算、通知投递和时间线维护逐步迁移到异步链路。

## 技术栈

### 后端

- Go：用于实现高并发 HTTP API 和 Worker。
- Gin：负责路由、中间件、参数绑定和 HTTP 响应。
- GORM：负责 MySQL 数据访问和模型映射。
- MySQL：保存用户、视频、评论、关注、点赞、通知、私信等核心业务数据。
- Redis：用于缓存、排行榜、限流、Token 状态、分片上传会话和 Feed 时间线。
- RabbitMQ：作为业务事件总线，用于解耦同步接口和后台处理。
- JWT：用于访问令牌和接口鉴权。

### 前端

- Vue 3：实现短视频 Feed、发布、登录和用户页面。
- Vite：提供开发构建能力。
- TypeScript：增强类型约束和可维护性。
- Pinia：管理登录用户、Token、Feed 状态和通知状态。
- Vue Router：管理登录、首页、发布页、视频详情、用户主页等路由。

### 部署

- Docker Compose：本地一键启动 MySQL、Redis、RabbitMQ、后端和前端。
- `.env`：管理数据库、缓存、消息队列、JWT 和文件存储配置。

## 后端分层设计

后端代码按入口、应用层、领域层、基础设施层和传输层拆分：

```text
backend/
  cmd/
    api/
    worker/
  internal/
    app/
    config/
    domain/
      user/
      video/
      feed/
      social/
      notification/
      message/
    infrastructure/
      mysql/
      redis/
      mq/
      storage/
    middleware/
    transport/
      http/
  migrations/
  tests/
```

### cmd

`cmd/api` 是 HTTP 服务入口，负责加载配置、初始化依赖、注册路由并启动服务。

`cmd/worker` 是后台任务入口，负责连接 RabbitMQ，消费业务事件并调用应用服务完成异步处理。

### config

集中读取环境变量和配置文件，包括服务端口、数据库连接、Redis 地址、RabbitMQ 地址、JWT 密钥和文件存储目录。

### app

应用层编排业务用例，例如注册登录、发布视频、点赞视频、发表评论、关注用户和拉取 Feed。应用层负责事务边界和跨领域协作，不直接依赖 HTTP 框架。

### domain

领域层按业务边界组织核心模型、仓储接口和领域规则：

- `user`：用户、资料、账号凭证和用户统计。
- `video`：视频、封面、标签、发布状态和视频统计。
- `feed`：Feed 查询、游标、热度和时间线。
- `social`：点赞、评论、关注等互动关系。
- `notification`：通知类型、通知状态和投递规则。
- `message`：私信会话和消息。

### infrastructure

基础设施层实现外部依赖访问：

- `mysql`：GORM 模型、仓储实现和事务处理。
- `redis`：缓存、排行榜、限流、Token 和上传会话实现。
- `mq`：RabbitMQ 生产者、消费者和事件序列化。
- `storage`：本地文件存储，后续可替换为 MinIO、OSS 或 S3。

### middleware

放置鉴权、请求日志、错误恢复、CORS、限流和请求 ID 等 HTTP 中间件。

### transport/http

HTTP 传输层负责路由注册、请求参数解析、响应格式和错误码映射。传输层不直接写业务规则，只调用应用层服务。

## 前端结构设计

前端代码按接口、状态、路由、页面、通用组件和业务特性拆分：

```text
frontend/
  src/
    api/
    stores/
    router/
    pages/
    components/
    features/
```

### api

封装后端 HTTP 请求，包括用户、视频、Feed、互动、通知和私信接口。统一处理 Token 注入、响应解包和错误转换。

### stores

使用 Pinia 管理跨页面状态：

- 当前登录用户和 Token。
- Feed 列表、游标和加载状态。
- 视频互动状态。
- 通知未读数和 SSE 连接状态。

### router

管理页面路由和登录态守卫。公开页面允许匿名访问，需要登录的页面在路由层拦截。

### pages

页面级组件包括登录页、注册页、首页 Feed、发布页、视频详情、用户主页、通知页和私信页。

### components

沉淀通用 UI 组件，例如视频播放器、头像、上传控件、评论列表、加载状态和空状态。

### features

按业务特性组织较复杂的前端逻辑，例如视频发布、互动操作、Feed 播放体验和通知连接。

## 数据存储设计

MySQL 保存强一致、需要持久化的核心数据。主要实体包括：

- `users`：用户账号和公开资料。
- `user_stats`：作品数、获赞数、粉丝数和关注数。
- `videos`：视频基础信息、存储路径、发布状态和作者。
- `video_stats`：点赞数、评论数、播放数和热度辅助字段。
- `video_tags` / `tags`：视频和话题标签关系。
- `likes`：用户点赞关系。
- `comments`：评论内容和删除状态。
- `follows`：关注关系。
- `notifications`：通知记录和已读状态。
- `messages` / `conversations`：私信消息和会话。
- `outbox_events`：后续用于保证数据库变更和消息发送一致性的事件表。

设计原则：

- 核心关系表使用唯一索引保证幂等，例如用户对视频的点赞关系、用户之间的关注关系。
- 统计字段单独维护，减少列表查询时的聚合成本。
- 高频列表按查询条件建立复合索引，例如 `videos(author_id, created_at)` 和 `comments(video_id, created_at)`。
- 删除优先使用软删除或状态字段，便于审计和恢复。

## 缓存设计

Redis 在不同阶段承担不同职责。

### 视频详情缓存

缓存视频详情、作者摘要和统计信息，降低热点视频对 MySQL 的压力。缓存 Key 示例：

```text
video:detail:{video_id}
```

更新策略采用先写数据库，再删除缓存。读请求缓存未命中时回源 MySQL，并设置短 TTL，避免长期脏数据。

### 排行榜

使用 Redis ZSET 维护点赞榜和热榜：

```text
rank:video:likes
rank:video:hot
```

点赞榜分数使用点赞数。热榜分数由点赞、评论、播放和时间衰减共同计算。

### Token 与会话

后续可以将 Refresh Token 或 Token 黑名单存入 Redis，实现登出、续签和异常登录控制。

### 限流

使用 Redis 对登录、上传、评论、点赞等接口做按 IP 或用户维度的限流。

### 分片上传会话

Redis 记录上传任务状态、已完成分片和过期时间。临时分片文件存储在本地临时目录。

## 消息队列设计

RabbitMQ 用于承载业务事件，避免所有工作都阻塞在 HTTP 请求内。主要事件包括：

- `video.published`：视频发布后更新 Feed 或标签索引。
- `video.liked`：点赞后更新统计、热度和通知。
- `video.unliked`：取消点赞后更新统计和热度。
- `comment.created`：评论后更新统计、热度和通知。
- `comment.deleted`：删除评论后更新统计。
- `user.followed`：关注后更新统计和通知。
- `user.unfollowed`：取消关注后更新统计。

### 阶段 4 当前实现状态

当前阶段 4 已先落地 RabbitMQ 和 Worker 骨架：

- `docker-compose.yml` 增加 RabbitMQ 和管理界面。
- `cmd/api` 在启动时尝试连接 RabbitMQ，连接失败时记录日志并继续提供核心 HTTP 服务。
- `social.Service` 在点赞、评论、关注数据库操作成功后发布互动事件。
- `cmd/worker` 作为独立进程消费统一业务事件队列，当前先完成事件解析、日志和后续处理入口。

当前骨架阶段不把完整统计更新、通知生成、60 分钟滑动窗口全部迁入 Worker，后续再在消费处理函数中逐步补齐。

### 消费原则

- 消费逻辑必须幂等，重复消息不能导致统计重复增加。
- 处理失败的消息进入重试流程，多次失败后进入死信队列或失败表。
- 消息体携带事件 ID、事件类型、发生时间和业务主键。
- 需要强一致的核心写入先落 MySQL，再通过 Outbox 模式可靠发送事件。

### MVP 取舍

MVP 阶段可以先同步更新统计，不引入 RabbitMQ。代码结构保留事件发布接口，后续再替换为真实消息队列实现。

## 文件存储设计

MVP 阶段使用本地文件系统，按资源类型和日期组织：

```text
storage/
  videos/
  covers/
  avatars/
  chunks/
```

文件上传需要校验：

- 文件大小。
- 文件 MIME 类型和扩展名。
- 存储路径是否越权。
- 文件名是否需要重新生成，避免用户输入直接成为路径。

后续可将 `storage` 基础设施接口替换为 MinIO、OSS 或 S3。应用层只依赖存储接口，不感知底层实现。

## Feed 设计

Feed 分为最新流、关注流、热榜、点赞榜和话题流。

### 最新流

MVP 阶段直接查询 MySQL：按 `created_at` 和 `id` 倒序进行游标分页。游标包含上一页最后一条记录的时间和 ID。

### 关注流

初期可查询用户关注列表后按作者过滤视频。数据量增大后改为写扩散：用户关注的作者发布视频时，将视频 ID 写入粉丝的 Redis 时间线。

### 热榜

热榜使用 Redis ZSET。热度分数可以按以下因素计算：

```text
score = like_count * 3 + comment_count * 5 + play_count * 1 - age_hours * decay_factor
```

具体权重后续根据展示效果调整。热度更新可由互动事件触发，也可由定时任务补偿重算。

### 点赞榜

点赞榜按点赞数排序，适合使用 Redis ZSET 增量更新。MySQL 中仍保存最终统计值，Redis 作为高性能排序视图。

### 话题流

话题流通过视频标签关系查询，按发布时间游标分页。热门话题后续可用 Redis ZSET 维护。

### 分页模型

所有 Feed 列表统一使用游标分页。接口返回数据列表、下一页游标和是否还有更多数据。游标应尽量不暴露内部实现，后续可以使用 base64 编码的结构体。

## 通知设计

通知分为持久化通知和实时推送两部分。

### 持久化通知

所有通知先写入 MySQL，确保用户离线后仍能查看。通知表保存接收者、触发者、通知类型、业务对象、内容摘要、是否已读和创建时间。

### 实时通知

登录用户建立 SSE 连接。服务端在生成通知后，如果用户在线，则向对应连接推送事件。用户不在线时只保留数据库通知。

### SSE 连接管理

后端维护用户 ID 到连接的映射。连接断开时清理映射。需要心跳消息防止中间代理关闭长连接。

### 可靠性取舍

SSE 只负责实时体验，不作为唯一可靠通道。可靠性由数据库通知列表保证。

## 安全设计

### 密码安全

密码使用 bcrypt 或同等级算法加密存储，不保存明文密码。

### JWT 鉴权

Access Token 用于接口鉴权，包含用户 ID、过期时间和签名。服务端中间件解析 Token 后注入当前用户上下文。

### 权限控制

- 用户只能修改自己的资料。
- 用户只能删除自己的评论，或在规则允许时删除自己视频下的评论。
- 用户只能管理自己发布的视频。
- 私信和通知只能由当前登录用户查看。

### 上传安全

上传文件不使用用户原始文件名作为最终路径。服务端限制文件大小和类型，并避免路径穿越。

### 参数校验

所有外部输入都需要校验，包括分页参数、用户名、密码、视频标题、评论内容和上传元数据。

## 限流设计

限流按接口风险分级：

- 登录和注册：按 IP 限制，降低撞库和爆破风险。
- 上传：按用户限制，避免占满磁盘和带宽。
- 点赞、评论、关注：按用户和资源限制，避免刷数据。
- SSE：限制单用户连接数量。

MVP 阶段可以先在应用内保留限流接口，后续使用 Redis 实现分布式限流。

## 部署架构

本地开发阶段通过独立进程启动前端和后端，MySQL、Redis、RabbitMQ 可通过 Docker Compose 启动。

完整部署阶段使用 Docker Compose 管理：

```text
services:
  mysql
  redis
  rabbitmq
  backend-api
  backend-worker
  frontend
```

后端 API 暴露 HTTP 端口，前端通过环境变量配置 API Base URL。上传文件在本地开发阶段挂载为 Docker volume，避免容器重启后文件丢失。

## 可靠性与扩展性

### 健康检查

后端提供 `/healthz`，用于检查服务进程是否可用。后续可扩展 `/readyz` 检查 MySQL、Redis 和 RabbitMQ 连接状态。

### 日志

后端记录请求日志、错误日志和 Worker 消费日志。关键事件需要包含 request ID 或 event ID，方便排查异步链路问题。

### 幂等

点赞、取消点赞、关注、取消关注、分片上传和消息消费都需要幂等设计，避免重复请求或重复消费导致状态错误。

### 降级

Redis 不可用时，视频详情可以回源 MySQL，排行榜接口返回明确错误或降级为空列表。RabbitMQ 不可用时，同步主链路应优先保证核心数据写入，并通过 Outbox 或失败事件表等待恢复后补发。

### 扩展方向

- 本地存储替换为对象存储。
- 热榜算法从简单加权升级为更完整的时间衰减模型。
- Feed 从数据库查询演进为 Redis 时间线。
- Worker 从单消费者扩展为按事件类型拆分多个消费者。
- 通知从单实例内存连接管理扩展为 Redis Pub/Sub 或独立网关。
