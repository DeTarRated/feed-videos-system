# 短视频 Feed 系统重构计划

## 背景

当前仓库 `E:\code\feedsystem_video` 是从 GitHub clone 的别人的项目。目标不是在原仓库里简单改名或复制代码，而是新建一个自己的项目，从需求、架构、接口、数据库、前后端实现、部署与测试逐步完成一个短视频 Feed 系统，用作后续面试项目。

计划新项目路径：

```text
E:\code\feed-videos-system
```

建议在新路径重新初始化 Git 仓库，让提交历史体现自己的设计和实现过程。

## 核心原则

1. 原项目只作为需求和功能参考，不直接复制代码、目录结构、README、页面实现或接口命名。
2. 新项目要先写文档，再写代码：先明确做什么、为什么这么做、怎么验证。
3. 面试项目重点不是堆功能，而是能解释架构选择、缓存策略、异步链路、失败处理和工程化能力。
4. 每个阶段都要能独立运行、独立提交，避免一次性堆完导致无法解释实现过程。

## 新项目推荐目录

```text
feed-videos-system/
  README.md
  docs/
    requirement.md
    architecture.md
    api.md
    database.md
    interview-notes.md
  backend/
  frontend/
  docker-compose.yml
```

后端正式开发后推荐结构：

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

前端正式开发后推荐结构：

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

## 原项目功能范围总结

原项目是 Go + Vue 3 的短视频 Feed 系统，主要功能包括：

### 账号模块

- 用户注册
- 用户登录
- JWT Access Token
- Refresh Token
- 登出
- 修改用户名
- 修改密码
- 上传头像
- 更新个人简介
- 用户主页信息
- 用户主页统计：作品数、获赞数、粉丝数、关注数

### 视频模块

- 视频普通上传
- 视频封面上传
- 视频发布
- 作者作品列表
- 视频详情
- 视频详情缓存
- 话题标签
- 静态资源访问

### 分片上传模块

- 初始化分片上传
- 上传分片
- 查询上传状态
- 合并分片
- 断点续传

### 点赞模块

- 点赞
- 取消点赞
- 判断是否已点赞
- 我的点赞视频列表
- 点赞事件异步落库
- 点赞数统计
- 热度更新
- 点赞通知

### 评论模块

- 发布评论
- 删除评论
- 评论列表
- 评论事件异步落库
- 评论数统计
- `@username` 提及通知
- 热度更新

### 关注模块

- 关注用户
- 取消关注
- 粉丝列表
- 关注列表
- 粉丝数统计
- 关注数统计
- 关注事件通知

### Feed 模块

- 最新视频流
- 关注流
- 点赞榜
- 热榜
- 话题流
- 游标分页
- Redis 缓存时间线
- 冷热分离思路
- 短视频沉浸式播放体验

### 消息与通知模块

- 私信发送
- 私信列表
- SSE 实时通知
- 点赞通知
- 评论通知
- 关注通知

### 基础设施与工程化

- MySQL 数据持久化
- Redis 缓存、排行榜、限流、Token 存储、分片上传会话
- RabbitMQ 事件总线
- Worker 异步消费点赞、评论、关注、热度、时间线、通知事件
- Docker Compose 一键启动
- 健康检查 `/healthz`
- pprof 性能分析
- 基础单元测试
- CI 工作流

## 新项目技术栈建议

后端：

- Go
- Gin
- GORM
- MySQL
- Redis
- RabbitMQ
- JWT
- Docker Compose

前端：

- Vue 3
- Vite
- TypeScript
- Pinia
- Vue Router

文件存储：

- 第一阶段使用本地磁盘
- 后续可扩展为 MinIO、OSS 或 S3

## 第一阶段要写的文档

### `docs/requirement.md`

目标：说明项目要实现什么功能，不写代码细节。

建议包含：

```md
# 需求文档

## 项目定位

## 用户角色

## 功能模块总览

## 账号模块

## 视频模块

## 分片上传模块

## Feed 模块

## 互动模块

## 通知模块

## 私信模块

## 后台任务与异步处理

## 非功能需求

## MVP 范围

## 后续扩展
```

### `docs/architecture.md`

目标：说明准备用什么架构实现，以及为什么这样设计。

建议包含：

```md
# 架构设计

## 总体架构

## 技术栈

## 后端分层设计

## 前端结构设计

## 数据存储设计

## 缓存设计

## 消息队列设计

## 文件存储设计

## Feed 设计

## 通知设计

## 安全设计

## 限流设计

## 部署架构

## 可靠性与扩展性
```

### 后续文档

`docs/api.md`：记录接口路径、请求参数、响应结构、鉴权要求。

`docs/database.md`：记录表结构、索引、字段含义、关系和统计字段。

`docs/interview-notes.md`：记录面试可讲点，包括架构取舍、缓存一致性、异步消息可靠性、Feed 设计、项目难点。

## 实现阶段路线

### 阶段 0：项目初始化

目标：建立自己的项目仓库和文档骨架。

任务：

- 创建 `E:\code\feed-videos-system`
- 初始化 Git 仓库
- 创建 `README.md`
- 创建 `docs/requirement.md`
- 创建 `docs/architecture.md`
- 创建 `docs/api.md`
- 创建 `docs/database.md`
- 创建 `docs/interview-notes.md`
- 写入第一版需求文档和架构文档

建议提交：

```bash
git add .
git commit -m "docs: 初始化项目需求与架构文档"
```

### 阶段 1：MVP 基础版

目标：项目能跑起来，完成最小短视频闭环。

后端任务：

- 初始化 Go 后端项目
- 配置 Gin 路由
- 配置 GORM + MySQL
- 实现用户注册、登录、JWT 鉴权
- 实现视频上传、封面上传、发布、列表、详情
- 实现本地文件存储
- 提供 `/healthz`

前端任务：

- 初始化 Vue 3 + Vite + TypeScript
- 配置路由和 Pinia
- 实现登录/注册页
- 实现首页视频流
- 实现视频发布页
- 实现视频详情或播放视图

验证：

- 能注册登录
- 能上传并发布视频
- 首页能刷到已发布视频
- Docker 或本地命令能启动前后端

### 阶段 2：互动版

目标：实现基础社交互动。

任务：

- 点赞、取消点赞、是否已点赞
- 评论发布、删除、列表
- 关注、取关
- 粉丝列表、关注列表
- 用户主页统计
- 作者作品列表
- 我的点赞视频列表

验证：

- 用户 A 能关注用户 B
- 用户 A 能点赞/评论用户 B 的视频
- 用户主页统计正确变化
- 前端展示点赞、评论、关注状态

### 阶段 3：Feed 与缓存版

目标：让项目体现后端性能设计。

任务：

- Redis 缓存视频详情
- Redis 存储登录 Token 或 Refresh Token
- Redis ZSET 实现热榜
- 实现最新流、关注流、点赞榜、热榜、话题流
- 实现游标分页
- 实现接口限流

验证：

- 重复请求视频详情能命中缓存
- 热榜按点赞/评论/时间等规则排序
- Feed 翻页没有明显重复和漏数据
- 限流规则生效

### 阶段 4：异步事件版

目标：引入 RabbitMQ 和 Worker，体现高并发互动链路。

任务：

- RabbitMQ 连接管理
- 点赞事件异步处理
- 评论事件异步处理
- 关注事件异步处理
- 热度更新 Worker
- Feed 时间线更新 Worker
- 死信队列或重试机制
- Outbox 模式，保证数据库变更和消息发送的一致性
- MQ 不可用时的降级策略

验证：

- 点赞/评论接口能快速返回
- Worker 能最终更新数据库统计
- MQ 暂时不可用时系统有明确降级行为
- 重启 Worker 后任务能继续消费

### 阶段 5：通知与体验版

目标：补齐实时通知、分片上传和私信。

任务：

- SSE 实时通知
- 点赞通知
- 评论通知
- 关注通知
- 私信发送和列表
- 分片上传初始化、上传、状态查询、合并
- 断点续传
- 前端沉浸式视频播放体验

验证：

- 登录用户能收到实时通知
- 大文件能分片上传并合并成功
- 刷新页面后能继续未完成上传
- 私信列表可用

### 阶段 6：工程化与面试打磨

目标：让项目可展示、可运行、可解释。

任务：

- Docker Compose 一键启动 MySQL、Redis、RabbitMQ、后端、前端
- 编写 `.env.example`
- 编写 README 启动说明
- 编写接口测试或 Postman Collection
- 添加核心单元测试
- 添加 GitHub Actions CI
- 添加 pprof 或基础监控说明
- 补充架构图
- 补充面试讲解文档

验证：

- 新机器 clone 后可以按 README 启动
- CI 能通过
- 核心接口测试能通过
- README 能清楚展示项目亮点

## 面试重点

面试时不要说“我重构了别人的项目”，应该说：

> 我参考短视频 Feed 场景，从零设计并实现了一个 Go + Vue 的全栈项目，覆盖用户、视频发布、互动、Feed、缓存、异步事件、实时通知和 Docker 部署。

重点讲这些：

1. 为什么使用 Go + Gin 做后端。
2. 为什么用 MySQL 存核心业务数据。
3. Redis 在项目中承担哪些角色：Token、缓存、排行榜、限流、分片上传会话、Feed 时间线。
4. RabbitMQ 解决什么问题：削峰、异步落库、通知解耦、热度更新。
5. 点赞和评论为什么可以异步，怎么保证最终一致性。
6. Feed 为什么要用游标分页。
7. 热榜分数如何设计。
8. SSE 为什么适合通知场景。
9. 分片上传如何支持断点续传。
10. 项目如何通过 Docker Compose 快速部署。

## 下一会话启动提示词

关闭当前会话后，在 `E:\code\feed-videos-system` 新开 Claude 会话，可以直接输入：

```text
这是我的新项目目录 E:\code\feed-videos-system。我想从零实现一个短视频 Feed 系统作为面试项目。请先创建项目骨架，并写入 docs/requirement.md 和 docs/architecture.md。参考 E:\code\feed-videos-system\plan.md 里的重构计划，但不要复制原项目代码。
```

如果新目录还不存在，可以让 Claude 先创建：

```text
请在 E:\code\feed-videos-system 初始化新项目目录，创建 README.md、docs/requirement.md、docs/architecture.md、docs/api.md、docs/database.md、docs/interview-notes.md，并先完成 requirement.md 和 architecture.md 的第一版内容。
```

## 当前会话注意事项

- 当前目录仍是原 clone 项目：`E:\code\feedsystem_video`。
- 新项目目标目录是：`E:\code\feed-videos-system`。
- 不建议在原项目继续写新代码。
- 原项目可作为需求参考，但新项目应使用自己的文档、接口设计、目录结构和实现。
