# 项目执行进度

> 本文档记录当前实现状态和推荐执行顺序。长期目标、完整功能范围和阶段路线见仓库根目录的 `plan.md`。

## 文档职责

- `plan.md`：项目总设计方案、完整需求范围、架构方向和长期阶段路线。
- `docs/requirement.md`：产品需求和功能边界。
- `docs/architecture.md`：技术架构、分层、存储、缓存和异步设计。
- `docs/progress.md`：当前阶段的任务清单、完成状态和下一步顺序。
- `docs/api.md`：接口契约和 Apifox 验证信息。
- `docs/database.md`：数据库表、字段和索引设计。
- `docs/interview-notes.md`：面试讲解要点和架构取舍。
- `docs/feed-cache-design.md`：阶段 3 Feed、缓存、热榜和限流详细设计。

状态说明：

- `[x]` 已完成并完成基本验证。
- `[~]` 已实现，等待格式化、测试或 Apifox 验证收口。
- `[ ]` 尚未完成。

## 阶段 0：项目初始化

- [x] 初始化 Git 仓库和基础目录。
- [x] 编写需求文档和架构文档。
- [x] 创建 API、数据库和面试笔记文档。
- [x] 添加 `CLAUDE.md`，约定 Go 命令执行目录和接口验证方式。
- [x] 添加 `.env.example` 和基础 Docker Compose 配置。

## 阶段 1：MVP 基础版

### 后端

- [x] Go module 和 API 服务入口。
- [x] Gin 路由、配置加载、CORS、日志和 `/healthz`。
- [x] MySQL 连接、用户表和视频表迁移。
- [x] 用户注册、登录和 JWT 鉴权。
- [x] 本地视频和封面上传。
- [x] 静态媒体资源访问。
- [x] 视频发布、最新 Feed、视频详情。
- [x] 用户主页统计接口。
- [x] 作者作品列表接口。

### 前端

- [x] Vue 3 + Vite + TypeScript + Pinia + Router。
- [x] 登录/注册页面。
- [x] Feed 页面。
- [x] 视频发布页面和真实文件上传。
- [x] 视频详情页面。
- [x] 用户主页、作品列表和统计展示。

### 工程与文档

- [x] API 文档整理为 Apifox 友好的表格式。
- [x] 数据库文档同步 users/videos。
- [x] 面试笔记同步 MVP 架构取舍。
- [x] 第一阶段代码已提交：`7f5ab75 feat: complete MVP foundation`。

## 阶段 2：互动版当前进度

当前按“点赞 -> 评论 -> 关注”的顺序进行。

### 第一步：点赞模块

- [x] 新增 `likes` 表和唯一约束 `(user_id, video_id)`。
- [x] 新增点赞领域模型、repository 和 service。
- [x] 新增点赞接口：点赞、取消点赞、点赞状态、我的点赞视频。
- [x] 接入 JWT 鉴权和 App 依赖注入。
- [x] 前端视频详情页增加点赞/取消点赞和点赞数。
- [x] API 文档补充点赞接口。
- [x] 已修复点赞状态查询的可选鉴权和刷新显示问题。
- [x] 点赞模块已形成阶段性代码范围，和评论模块一起提交。

### 第二步：评论模块

- [x] 新增 `comments` 表、索引和删除状态。
- [x] 实现发表评论、评论列表和删除评论接口。
- [x] 更新视频评论数，详情接口返回真实 `commentCount`，前端发表评论和删除后同步更新。
- [x] 前端详情页增加评论列表和发表评论表单。
- [x] 补 API、数据库和面试文档。
- [x] 使用 Apifox 验证评论流程：发表评论、列表查询、本人删除、权限校验和重复删除。
- [x] 在 `backend` 目录执行 `gofmt` 和 `go test`。

### 第三步：关注模块

- [x] 新增 `follows` 表和唯一约束 `(follower_id, following_id)`。
- [x] 实现关注、取消关注、关注状态接口。
- [x] 实现粉丝列表和关注列表。
- [x] 禁止关注自己，并保证关注/取关幂等。
- [x] 用户主页统计接入真实粉丝数和关注数。
- [x] 前端用户主页增加关注交互。
- [x] 补 API、数据库和面试文档。

### 第四步：用户主页与账号管理

- [x] 新增用户资料接口：公开资料、当前登录用户资料、资料更新和密码修改。
- [x] 实现账号资料管理：修改昵称、头像、简介、密码和登出。
- [x] 重构用户主页为顶部资料区和内容 Tab：作品、喜欢、粉丝、关注。
- [x] 前端主页参考 TikTok 官网结构，补充资料展示、关注交互、编辑资料和账号管理状态。
- [x] 复用现有 `users.nickname`、`users.avatar_url`、`users.bio` 字段，暂不新增用户资料表。
- [x] 用户名暂不开放修改，昵称允许重复，用户名继续保持唯一约束。
- [x] 补 API、进度文档和面试文档，等待 Apifox 和前端手动验证收口。

## 阶段 3：Feed 与缓存版当前进度

当前按“Redis 基础设施 -> 视频详情缓存 -> 最新流游标分页 -> 热榜 -> 限流”的顺序进行。详细设计见 `docs/feed-cache-design.md`。

### 第一步：Redis 基础设施

- [x] 在 `docker-compose.yml` 增加 Redis 服务。
- [x] 在后端配置中增加 Redis 地址、密码、DB、缓存 TTL 等配置项。
- [x] 新增 Redis 客户端初始化和关闭逻辑。
- [x] 后端启动时完成 Redis 连接检查或明确降级策略。
- [x] 补充 `.env.example` 中的 Redis 配置。

### 第二步：视频详情缓存

- [x] 为 `GET /api/v1/videos/:id` 增加 Redis 缓存读取。
- [x] 未命中缓存时从 MySQL 查询并写入 Redis。
- [x] 设计缓存 Key、TTL 和序列化结构。
- [x] 发布、修改或删除视频时明确缓存失效策略。
- [x] 增加命中缓存和未命中缓存的验证方式。

### 第三步：最新流游标分页

- [x] 将视频列表从简单 `limit` 查询升级为游标分页。
- [x] 使用 `created_at DESC, id DESC` 保证排序稳定。
- [x] 响应中返回 `nextCursor`，请求中支持 `cursor` 和 `limit`。
- [x] 前端 Feed 页面支持继续加载下一页。
- [x] 验证翻页时没有明显重复和漏数据。

### 第四步：热榜

- [x] 使用 Redis ZSET 存储视频热度分数。
- [x] 设计热度分数公式，先覆盖点赞数、评论数和发布时间衰减。
- [x] 新增热榜查询接口。
- [x] 点赞、取消点赞、评论、删除评论后同步更新热度分数。
- [x] 前端增加热榜入口或展示区域。

### 第五步：接口限流

- [x] 基于 Redis 实现固定窗口限流中间件。
- [x] 优先保护登录、发布、点赞、评论和关注等高频写接口。
- [x] 返回明确的 429 响应和错误信息。
- [x] 文档记录每类接口的限流规则。
- [x] 用 Apifox 验证限流规则生效。

### 阶段 3 Apifox 验证接口清单

建议先在 Apifox 环境变量中配置：

| 变量名 | 示例值 | 说明 |
| --- | --- | --- |
| `baseUrl` | `http://localhost:8080/api/v1` | API 基础地址 |
| `token` | 登录后保存 JWT | 需要登录的接口使用 |
| `videoId` | 已发布的视频 ID | 用于详情、点赞、评论和热榜验证 |
| `commentId` | 已发布的评论 ID | 用于删除评论验证 |
| `nextCursor` | 最新流第一页返回值 | 用于验证第二页 |

#### 1. 健康检查

| 项目 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/healthz` |
| 鉴权 | 无 |
| Body | 无 |
| 检查点 | 返回 `status: ok`，确认后端已启动。Redis 不可用时，阶段 3 当前策略是后端启动失败。 |

#### 2. 登录获取 Token

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/auth/login` |
| 鉴权 | 无 |
| Headers | `Content-Type: application/json` |
| Body | `{"username":"你的用户名","password":"你的密码"}` |
| 检查点 | 返回 `token` 和 `user`，把 token 保存到 Apifox 环境变量 `token`。 |
| 阶段 3 关联点 | 登录接口已挂 IP 限流，每 IP 每分钟 10 次。 |

#### 3. 视频详情缓存

| 项目 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/videos/{{videoId}}` |
| 鉴权 | 无 |
| Body | 无 |
| 检查点 | 返回视频基础字段和 `commentCount`。连续请求两次，响应结构应一致；第二次应走 Redis 缓存命中路径。 |
| 阶段 3 关联点 | 验证 `video:detail:{videoId}` 缓存读取和回填。 |

#### 4. 视频详情缓存失效：点赞

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/videos/{{videoId}}/like` |
| 鉴权 | 必须 |
| Headers | `Authorization: Bearer {{token}}` |
| Body | 无 |
| 检查点 | 返回 `liked: true` 和新的 `likeCount`。随后再次请求 `GET /videos/{{videoId}}`，详情缓存应被删除并重新回填，统计不应长期停留旧值。 |
| 阶段 3 关联点 | 点赞后删除视频详情缓存，并同步更新热榜分数。 |

#### 5. 视频详情缓存失效：取消点赞

| 项目 | 内容 |
| --- | --- |
| 方法 | `DELETE` |
| URL | `{{baseUrl}}/videos/{{videoId}}/like` |
| 鉴权 | 必须 |
| Headers | `Authorization: Bearer {{token}}` |
| Body | 无 |
| 检查点 | 返回 `liked: false` 和新的 `likeCount`。随后再次请求视频详情，统计不应长期停留旧值。 |
| 阶段 3 关联点 | 取消点赞后删除视频详情缓存，并同步更新热榜分数。 |

#### 6. 视频详情缓存失效：发表评论

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/videos/{{videoId}}/comments` |
| 鉴权 | 必须 |
| Headers | `Content-Type: application/json`，`Authorization: Bearer {{token}}` |
| Body | `{"content":"阶段 3 缓存验证评论"}` |
| 检查点 | 返回评论对象，记录 `commentId`。随后请求视频详情，`commentCount` 应更新。 |
| 阶段 3 关联点 | 评论后删除视频详情缓存，并同步更新热榜分数。 |

#### 7. 视频详情缓存失效：删除评论

| 项目 | 内容 |
| --- | --- |
| 方法 | `DELETE` |
| URL | `{{baseUrl}}/videos/{{videoId}}/comments/{{commentId}}` |
| 鉴权 | 必须 |
| Headers | `Authorization: Bearer {{token}}` |
| Body | 无 |
| 检查点 | 返回 `deleted: true`。随后请求视频详情，`commentCount` 应更新。 |
| 阶段 3 关联点 | 删除评论后删除视频详情缓存，并同步更新热榜分数。 |

#### 8. 最新流第一页

| 项目 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/videos?limit=5` |
| 鉴权 | 无 |
| Body | 无 |
| 检查点 | 返回 `items`、`nextCursor`、`hasMore`。如果 `hasMore` 为 `true`，保存 `nextCursor`。 |
| 阶段 3 关联点 | 验证最新流游标分页第一页。 |

#### 9. 最新流第二页

| 项目 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/videos?limit=5&cursor={{nextCursor}}` |
| 鉴权 | 无 |
| Body | 无 |
| 检查点 | 第二页不应重复第一页最后一个视频；数据不足时 `hasMore` 为 `false`，`nextCursor` 为空字符串。 |
| 阶段 3 关联点 | 验证 `published_at DESC, id DESC` 稳定游标分页。 |

#### 10. 热榜视频

| 项目 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/feed/hot?limit=10` |
| 鉴权 | 无 |
| Body | 无 |
| 检查点 | 返回 `items`。对某个视频点赞或评论后再次请求，目标视频应进入热榜或排序发生变化。ZSET 为空时会降级返回最新视频。 |
| 阶段 3 关联点 | 验证 Redis ZSET `rank:video:hot`。 |

#### 11. 登录限流

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/auth/login` |
| 鉴权 | 无 |
| Headers | `Content-Type: application/json` |
| Body | `{"username":"你的用户名","password":"你的密码"}` |
| 检查点 | 同一 IP 1 分钟内连续快速请求超过 10 次后，返回 HTTP `429` 和 `too many requests`。 |
| 阶段 3 关联点 | 验证 IP 维度固定窗口限流。 |

#### 12. 注册限流

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/auth/register` |
| 鉴权 | 无 |
| Headers | `Content-Type: application/json` |
| Body | `{"username":"limit_test_随机数","password":"123456","nickname":"限流测试"}` |
| 检查点 | 同一 IP 1 分钟内连续快速请求超过 5 次后，返回 HTTP `429` 和 `too many requests`。用户名需要每次不同，避免被唯一索引错误干扰。 |
| 阶段 3 关联点 | 验证注册接口 IP 限流。 |

#### 13. 点赞限流

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` 或 `DELETE` |
| URL | `{{baseUrl}}/videos/{{videoId}}/like` |
| 鉴权 | 必须 |
| Headers | `Authorization: Bearer {{token}}` |
| Body | 无 |
| 检查点 | 同一用户 1 分钟内连续快速请求超过 60 次后，返回 HTTP `429` 和 `too many requests`。 |
| 阶段 3 关联点 | 验证用户 ID 维度写接口限流。 |

#### 14. 评论限流

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/videos/{{videoId}}/comments` |
| 鉴权 | 必须 |
| Headers | `Content-Type: application/json`，`Authorization: Bearer {{token}}` |
| Body | `{"content":"评论限流测试"}` |
| 检查点 | 同一用户 1 分钟内连续快速请求超过 20 次后，返回 HTTP `429` 和 `too many requests`。 |
| 阶段 3 关联点 | 验证评论写接口限流。 |

#### 15. 发布视频限流

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/videos` |
| 鉴权 | 必须 |
| Headers | `Content-Type: application/json`，`Authorization: Bearer {{token}}` |
| Body | `{"title":"限流测试视频","description":"发布限流验证","videoUrl":"/media/videos/test.mp4","coverUrl":""}` |
| 检查点 | 同一用户 1 分钟内连续快速请求超过 5 次后，返回 HTTP `429` 和 `too many requests`。如果验证真实发布流程，建议先使用上传接口拿到真实 `videoUrl`。 |
| 阶段 3 关联点 | 验证发布写接口限流。 |

#### 16. 关注限流

| 项目 | 内容 |
| --- | --- |
| 方法 | `POST` 或 `DELETE` |
| URL | `{{baseUrl}}/users/{{targetUserId}}/follow` |
| 鉴权 | 必须 |
| Headers | `Authorization: Bearer {{token}}` |
| Body | 无 |
| 检查点 | 同一用户 1 分钟内连续快速请求超过 30 次后，返回 HTTP `429` 和 `too many requests`。`targetUserId` 不能是当前登录用户自己。 |
| 阶段 3 关联点 | 验证关注写接口限流。 |

1. 完成阶段 3 设计文档，明确 Redis 使用边界、Key 设计和验证清单。
2. 接入 Redis 基础设施，并保证后端可以正常启动和测试。
3. 先实现视频详情缓存，形成阶段 3 的第一个可验证性能功能。
4. 实现最新流游标分页，再逐步补热榜和限流。
5. 每完成一个阶段 3 子模块，同步更新 API、数据库和面试文档后再提交。

## 当前阶段完成标准

阶段 3 只有同时满足以下条件才算完成：

- Redis 基础设施可用，配置清晰，并支持本地 Docker Compose 启动。
- 视频详情接口能读写缓存，并有明确缓存失效策略。
- 最新 Feed 支持稳定游标分页。
- 热榜能按 Redis ZSET 分数返回视频列表。
- 高频写接口具备基础限流能力。
- Apifox、前端手动验证和必要 Go test 已完成。
- API、数据库、面试文档与实际实现一致。
- 阶段 3 代码已形成独立 Git 提交。
