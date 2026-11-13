# 热点视频高并发改造实施 Spec

> 状态：Backend implemented; code review passed for Milestones A-E. Load/chaos testing requires a running environment and remains as follow-up.  
> 上游设计：`.agent/hot-video-high-concurrency-design.md`  
> 目标：将架构设计拆成可编码、可迁移、可验证的 Go 后端改造任务。  
> 本文只定义目标实现，不表示功能已经落地。

## 1. 背景与现状证据

当前后端已经具备 API、Worker、RabbitMQ、Redis、MySQL 和热榜窗口的基础代码，但仍存在以下问题：

- `backend/internal/infrastructure/mq/rabbitmq.go` 每次发布都创建 Channel 并声明 Exchange。
- API 在 RabbitMQ 不可用时可能退化为无发布器，`social.Service.publish` 会返回 nil，调用方无法区分“已接收”和“未处理”。
- `backend/cmd/worker/main.go` 当前顺序消费消息，没有 Worker Pool、prefetch 和微批次。
- 点赞持久化由脏视频集合驱动，每轮读取完整点赞集合并执行批量 UPSERT 与 `NOT IN` 清理。
- 评论创建仍同步逐条插入 MySQL；评论计数依赖 Redis 增量，重复消息可能重复累计。
- 点赞回源使用进程内 singleflight，但视频详情回源没有相同资源合并和有界回源。
- Worker 进程同时启动窗口聚合与清理任务，Worker 扩容会产生重复调度。
- 热榜后续分页仍先读取实时 Top N，再通过 `lastVideoID` 遍历快照定位。

## 2. 改造目标

1. 互动写接口采用可靠的 MQ-first Command，Publisher Confirm 成功后才返回 Accepted。
2. Worker 使用有界并发、prefetch 和微批次处理点赞与评论。
3. MySQL 通过唯一键、操作版本和事务保证至少一次投递下的幂等与计数正确。
4. Redis 更新具备幂等和乱序保护，避免重复计数与重复热度。
5. 视频元信息、动态统计、用户点赞状态和评论缓存分离。
6. 缓存回源具备 singleflight、跨实例重建控制、并发上限和超时。
7. 热榜窗口由独立 Scheduler 主备单活执行，分钟桶通过原子检查点保证幂等。
8. 热榜分页使用快照游标，并通过 MGET 和缺失项回源组装页面。
9. 建立单元测试、集成测试、故障注入和压测入口，不编造容量指标。

## 3. 范围与交付边界

### 3.1 本期包含

- 点赞、取消点赞、评论创建、评论删除的异步命令链路。
- RabbitMQ 发布确认、通道复用、手动 ACK、有限重试和 DLQ。
- 点赞微批次条件写入；评论按视频分组批量写入。
- `video_stats` 和必要的数据库迁移。
- 视频详情与统计数据拆分、热点缓存回源保护。
- `cmd/worker` 与 `cmd/scheduler` 职责拆分。
- Redis 单活锁、续约、ownerToken 和分钟桶幂等检查点。
- 热榜快照 offset 游标和页面批量缓存读取。
- 后端 API、前端 pending 状态适配及必要文档。
- 可重复的验证与故障注入脚本或测试入口。

### 3.2 本期不包含

- Kafka、Flink、etcd、ZooKeeper、Raft 或跨地域多活。
- 关注、私信、通知的全链路异步化。
- 通用推荐算法、视频转码和对象存储迁移。
- 未经测量就引入 Redis Cluster 或固定大规模分片。
- 声称 Exactly Once 或生产级容量已经验收。

## 4. 目标进程与代码职责

```text
cmd/api
  └── HTTP、鉴权、限流、命令构造、幂等请求处理、Publisher Confirm

cmd/worker
  └── like/comment 队列、有界 Worker Pool、微批次、MySQL/Redis、ACK/retry

cmd/scheduler
  └── 热榜窗口单活调度、分钟桶检查点、清理和补跑
```

目标包职责：

- `internal/domain/social`：Command 模型、业务校验、操作版本和响应模型。
- `internal/infrastructure/mq`：连接恢复、拓扑、复用发布器、Confirm、消费与重试。
- `internal/infrastructure/mysql`：事务批次、条件 UPSERT、评论批量写入、统计与对账。
- `internal/infrastructure/cache`：在线状态、统计、缓存重建、热榜窗口、Scheduler 锁与检查点。
- `internal/application` 或现有 `internal/app`：依赖装配和生命周期，不在 HTTP Handler 中实现基础设施细节。

## 5. API 与兼容性 Spec

### 5.1 写接口幂等键与不可变 Command

点赞、取消点赞、评论创建和评论删除要求客户端携带 `Idempotency-Key`。同一逻辑操作重试必须复用该值。

Redis 幂等记录以 `actorId + operation + idempotencyKey` 为键，保存：

- 请求指纹，覆盖接口、业务键和规范化请求体。
- 完整且不可变的 Command。
- `operationId`、`commentId`（如有）和 `opVersion`。
- 实际待发布的消息字节和 schemaVersion。

首次请求与并发重试按以下流程处理：

1. 已存在记录时先校验指纹；不同内容返回 409，相同内容直接取出原 Command。
2. 不存在记录时生成 Command，并通过 Redis `SET NX` 原子保存完整记录。
3. `SET NX` 竞争失败时丢弃本地生成的 Command，读取获胜记录；版本序列允许出现空洞，不允许同一幂等键出现两个有效 Command。
4. 发布时使用记录中的原始消息字节。Confirm 成功、失败或结果未知都不修改 Command 身份。
5. Confirm 结果未知或客户端超时时，重试只重新发布同一条 Command，由数据库 outcome 和 Redis 效果标记去重。

幂等记录 TTL 覆盖客户端最大重试窗口。首版不依赖记录中的 Confirm 状态决定是否安全重发，重复发布始终由下游幂等处理。

Redis 全量数据丢失不是普通冷缓存：如果幂等记录丢失，先查询 MySQL outcome；能证明已提交时按原身份重建，无法证明时返回 503 并进入恢复流程，禁止重新生成版本继续发布。

### 5.2 点赞与取消点赞

保持现有 URL：

- `POST /api/v1/videos/:id/like`
- `DELETE /api/v1/videos/:id/like`

Confirm 成功后返回 HTTP 202：

```json
{
  "data": {
    "videoId": 2001,
    "liked": true,
    "status": "pending",
    "operationId": "01...",
    "opVersion": "123"
  }
}
```

不再由 API 返回未经确认的 `likeCount`。读取计数继续使用状态查询接口。

### 5.3 评论创建与删除

保持现有 URL：

- `POST /api/v1/videos/:id/comments`
- `DELETE /api/v1/videos/:id/comments/:commentId`

创建评论 Confirm 成功后返回 HTTP 202，响应包含完整 pending 评论、`commentId`、`operationId` 和 `createdAt`。

评论 ID 采用 **UUID 字符串**，在 API 发布前生成：

- Go `Comment.ID` 和相关请求/响应模型改为字符串。
- MySQL `comments.id` 迁移为 `VARCHAR(36)`，旧自增数值按字符串保留。
- Handler 不再使用 `ParseUint` 解析评论 ID。
- 前端评论 ID 全部按字符串处理，避免宽整数 JSON 精度问题。
- 不引入 Snowflake 节点分配、时钟回拨或跨节点时钟依赖。

删除已提交评论时返回 202 与 pending 状态，Worker 再次校验作者和状态版本。**首版禁止删除尚未完成创建的 pending 评论**：

- 返回可重试的 HTTP 409，错误码为 `comment_not_ready`。
- 前端对 pending 评论禁用删除，并提供状态刷新。
- 首版不实现 delete-before-create tombstone。
- 创建提交后，删除命令通过同一 commentId 的集中操作版本保证晚于创建。
- Worker 必须重新校验作者，不能只信任 API 的预校验。

### 5.4 失败语义

- Confirm 失败、超时或拓扑未就绪：返回 503 或可重试的限流响应。
- 参数或鉴权失败：维持 4xx。
- 相同幂等键对应不同请求体：返回 409。
- MQ 不可用时不得返回 Accepted。
- Redis 是操作版本或幂等记录的必要依赖时，对应写接口明确降级为不可用，不绕过保护。

### 5.5 前端适配

- 所有异步写请求生成并复用 Idempotency-Key。
- 点赞使用响应目标状态乐观展示。
- 评论创建将 pending 评论插入当前用户视图，pending 状态禁用删除。
- 点赞状态和评论读模型携带当前 `opVersion`，前端只有读到不低于 pending 操作的版本后才清除 pending。
- 删除 pending 评论收到 `comment_not_ready` 时保留本地状态并允许稍后重试。
- 失败时保留重试能力，不重新生成逻辑操作身份。

## 6. Command 模型与版本生成

统一 Command 至少包含：

```text
operationId、idempotencyKey、requestFingerprint
opVersion、type、actorId、videoId
commentId、content、occurredAt、schemaVersion
```

### 6.1 身份与版本

- `operationId` 和 `commentId` 使用 UUID 字符串。
- `opVersion` 使用 uint64 集中序列，在 JSON 中编码为字符串。
- 点赞业务键为 `(actorId, videoId)`，使用 `like:seq:{actorId}:{videoId}`。
- 新评论创建的版本固定为 1。
- 已提交评论的删除使用 `comment:seq:{commentId}`，从数据库当前版本初始化后递增。
- Command 使用显式 `schemaVersion`，未知版本进入不可重试错误路径。
- 消息体不携带认证凭证。

集中序列保证同一状态业务键的因果顺序，不使用 Snowflake 作为操作版本，因此跨 API 节点不依赖时钟偏斜来判断新旧操作。

### 6.2 正常冷键初始化

序列 Key 不设置 TTL，并纳入禁止逐出的元数据 Key 空间。正常首次使用或冷缺失时：

1. 以业务键执行 singleflight，合并并发初始化。
2. 查询 MySQL 中该业务键最后提交的 `op_version`；无记录时为 0。
3. 通过 Lua `SET NX` 初始化序列底值。
4. 对初始化后的序列执行 INCR，得到本次版本。
5. 并发初始化由 singleflight 和 SET NX 共同去重。

正常热路径不访问 MySQL。冷初始化回源必须有并发上限和超时。

### 6.3 Redis 数据丢失恢复

普通冷键初始化不能安全处理 Redis 全量丢失，因为 MySQL 可能尚未包含已被 MQ 接受、仍在处理中的高版本命令。

- 部署上禁止序列和幂等元数据被 LRU/LFU 策略逐出。
- 一旦确认发生 Redis 数据丢失，互动写进入 fail-closed 恢复状态。
- 停止新写入并让 Worker 有界排空或暂停，依据 MySQL 关系、outcome 和最大已提交版本重建序列底值。
- 对无法证明身份的幂等重试返回 503，不重新生成 Command。
- 完成关系、统计、热度和序列核对后再开放写入。

该取舍主动降低故障时可用性，以避免静默破坏顺序或重复评论。

## 7. RabbitMQ Spec

### 7.1 拓扑

至少拆分：

- `interaction.command` Exchange。
- 点赞 Command Queue 与评论 Command Queue。
- 对应 Retry Queue 与 DLQ。
- 明确 routing key、queue arguments、DLX 和重试 TTL。

点赞与评论是否允许跨队列乱序必须由第 6 节的版本方案兜底，不能依赖部署时恰好单消费者。

### 7.2 发布器

- 复用 Connection 和受控 Channel。
- 开启 Publisher Confirm，并处理 Confirm 超时、连接断开和 Channel 关闭。
- 不为每条消息创建 Channel 或重复声明 Exchange。
- 发布并发受配置限制。
- 拓扑只在启动或恢复时声明。
- 重连后恢复 Confirm 状态和队列拓扑。

### 7.3 消费者

- 手动 ACK。
- 设置 prefetch。
- 按队列配置有界 Worker Pool。
- 永久格式错误进入 DLQ，不无限 requeue。
- 临时错误有限重试，超过上限进入 DLQ。
- 记录 `operationId`、重试次数和失败原因。

### 7.4 ACK 边界

一个微批次只有在以下步骤全部完成后才能 ACK：

1. 必要的 MySQL 事务提交。
2. 必要的 Redis 状态、统计和热度更新成功。
3. 本批所有消息均已成功处理或被明确路由到 retry/DLQ。

部分子批次失败时，可以重试整个原始批次，但所有外部效果必须幂等。禁止仅因部分视频成功就 ACK 整批消息。

## 8. Worker 微批次 Spec

### 8.1 批次形成

- 达到配置批大小或最大等待时间即处理。
- 单批命令数和总字节数有上限。
- 处理超时、取消和 Worker 关闭有明确语义。
- 关闭时停止拉取新消息，在有界时间内完成或 requeue 未完成任务。

### 8.2 事务性 outcome 与统一重放协议

每个 Command 都必须有可持久化、可重放的 `interaction_outcome`。首处理和重放使用同一协议：

1. 按 `operationId` 查询 outcome 并校验请求指纹。
2. 已存在 outcome 时，不重新计算或修改 MySQL，直接读取上次提交结果。
3. 不存在时，在同一 MySQL 事务内完成业务变更、统计绝对值与 `stats_version` 更新、outcome 写入。
4. outcome 保存结果状态、统计绝对值与版本、热度效果及所属分钟。
5. Worker 根据 outcome 重放 Redis，而不是根据本次数据库是否实际改行猜测 Redis 是否执行过。
6. Redis 全部必要效果完成后才 ACK。

批内被新版操作合并的旧命令也要记录 `superseded` outcome；旧消息晚到时只生成或读取无效果 outcome，不产生统计和热度。

### 8.3 点赞批处理

1. 按 `(actorId, videoId)` 保留最高 `opVersion`。
2. 按 `videoId` 分组，限制单事务大小，并按业务键确定顺序加锁。
3. 已存在 outcome 的命令走统一重放协议。
4. 新命令在事务中读取并锁定相关点赞关系，过滤旧版本。
5. 计算真实状态变化，批量条件 UPSERT 点赞关系。
6. 更新统计绝对值和 `stats_version`，写入每个命令的 outcome。
7. 提交后按 opVersion 更新 Redis 在线关系，按 stats_version 覆盖统计，并按 outcome 应用热度。

同键跨批次乱序由数据库和 Redis 的版本比较处理；不存在行时的并发插入、死锁和重试必须有集成测试。

### 8.4 评论创建批处理

1. 按 `videoId` 分组，每个事务只处理一个视频的有界批次。
2. 查询并校验相同 `commentId` 的既有记录与请求指纹，冲突进入不可重试错误路径。
3. 对单个视频执行一次多行 `INSERT IGNORE`，以实际插入行数作为该视频新增评论数。
4. 在同事务中更新统计绝对值、`stats_version` 和每个命令的 outcome。
5. 提交后按 outcome 批量维护评论详情、首页缓存、统计和热度。
6. 重放读取既有 outcome，不重复插入、计数或加分。

`INSERT IGNORE RowsAffected` 的含义必须用当前 MySQL 驱动、DSN 和 GORM 路径做集成测试；禁止将跨视频总 RowsAffected 当作各视频增量。

### 8.5 评论删除批处理

- API 只接受已提交评论的删除，pending 创建返回 409。
- 按视频分组并执行带作者、状态和版本条件的有界批量软删除。
- 仅首次从有效状态变为删除状态时扣减统计。
- 删除版本必须高于创建版本，Worker 再校验作者。
- 每次处理仍写入或读取 outcome，重复删除直接重放原效果。

### 8.6 Redis 失败重放

MySQL 提交后 Redis 失败会触发原命令重放。实现必须保证：

- 数据库重放只读取 outcome，不重复修改业务行或统计。
- Redis 点赞状态按 opVersion 幂等。
- Redis 统计只在 stats_version 更高时覆盖绝对值；相同版本已存在时直接跳过。
- 热度使用 outcome 的 operationId 作为唯一效果 ID，原子去重后应用。
- 已过窗口的迟到命令只完成必要持久化，不向当前分钟桶注入旧热度。
- outcome 中的 heatMinute 来自原命令业务时间，而非重试时间。

## 9. MySQL 迁移 Spec

新增迁移，不修改已经部署的旧迁移文件。

### 9.1 likes

增加：

- `op_version`，用于条件更新。
- 必要的版本与状态索引。

现有行迁移为可识别的初始版本，之后只允许更高版本覆盖。

### 9.2 comments

- 将 `id` 从自增 BIGINT 迁移为 `VARCHAR(36)`，旧数值 ID 转为字符串保留。
- 新评论使用 API 预生成 UUID，不再依赖自增。
- 增加 `op_version`、创建 `operation_id` 和请求指纹。
- `id` 主键继续保证评论唯一，operation_id 与指纹用于核对重放和冲突。
- Go 与前端模型同步改为字符串 ID。

### 9.3 video_stats

至少包含：

- `video_id` 主键。
- `like_count`、`comment_count` 绝对值。
- 单调递增的 `stats_version`。
- `updated_at`。

统计与业务变更、outcome 在同事务更新。计数回源读取 `video_stats`，不再对 likes/comments 执行常规 COUNT(*) 聚合。

### 9.4 interaction_outcomes

首版强制使用事务性 outcome 表，至少包含：

- `operation_id` 主键和 `request_fingerprint`。
- `command_type`、`business_key`、`op_version`、`video_id`。
- `db_outcome`（applied / duplicate / superseded 等）和结果状态。
- 提交时的 `like_count`、`comment_count`、`stats_version`。
- `heat_delta`、`heat_minute`。
- `created_at`。

业务变更、统计和 outcome 必须在同一事务。重放只读取既有 outcome。首版不清理 outcome，避免清理窗口削弱重放与 Redis 重建证明；后续若增加清理，必须先定义独立的长期效果归档与重建边界。

## 10. Redis 状态与缓存 Spec

### 10.1 数据分离

- `video:detail:{videoId}`：元信息，不随点赞评论失效。
- `video:stats:{videoId}`：点赞数、评论数和必要版本。
- 用户点赞状态独立保存。
- 评论详情与评论首页独立保存。
- 热榜分钟桶、聚合 ZSET 和快照独立保存。

### 10.2 点赞状态、统计与热度原子更新

Worker 根据持久化 outcome 调用有界 Lua：

1. 点赞关系按 opVersion 拒绝旧操作。
2. 统计按 stats_version 接收 outcome 中的绝对值；旧版本不能覆盖新版本。
3. 相同 stats_version 且统计已存在时直接跳过，Redis 重建后版本缺失则允许写入。
4. 热度使用 `hot:applied:{operationId}` 去重，效果标记与分钟桶/聚合榜更新原子完成。
5. 返回实际应用或跳过的原因，供 Worker 日志与指标记录。

热度效果标记保留时间覆盖 60 分钟窗口和最大命令重放周期。Redis 热度全量重建时，从 outcome 中当前窗口内的 heatMinute/heatDelta 重放，不依赖仍然存在的 Redis 标记。

### 10.3 评论缓存

- 创建成功后批量写详情。
- 首页按批追加或重建，限制长度。
- 删除成功后批量移除或标记删除。
- 重放不得重复追加同一评论。
- 缓存维护失败必须进入可重试路径，不能仅记录日志后 ACK。

### 10.4 点赞集合分片

本期默认不启用大规模分片。只有压测或实际数据证明 Big Key、迁移或单 Key 瓶颈后，才加入固定分片；Spec 不以预设规模触发分片。

## 11. 热点读保护 Spec

### 11.1 视频详情

- 元信息缓存 TTL 加随机抖动。
- 本进程使用 singleflight。
- 多实例重建使用短 TTL Redis 锁与 ownerToken。
- 有旧值时允许 stale-while-revalidate。
- 不存在视频使用短 TTL 空值缓存。
- 回源数据库有独立信号量和超时。

### 11.2 点赞状态与统计

- 正常路径读 Redis。
- 回源点赞关系走唯一索引，统计读 `video_stats`。
- 回源有 singleflight 与有界并发。
- Redis 故障时不允许无界数据库 fallback。

### 11.3 评论读取

- 首页缓存缺失时使用 singleflight。
- 批量缓存维护与回源重建不能产生重复项。
- 其他评论页仍可使用 MySQL 稳定排序，暂不扩展为复杂缓存体系。

### 11.4 降级

- 元信息有旧值时优先旧值。
- 动态统计无法加载时允许明确降级。
- 所有外部调用有超时。
- 禁止在 Redis/MQ 故障时切换为逐请求同步写 MySQL。

## 12. Scheduler 单活与热榜 Spec

### 12.1 进程拆分

- `cmd/worker` 不再启动窗口聚合与清理循环。
- `cmd/scheduler` 负责窗口聚合、过期桶移除和榜单清理。
- Worker 可扩容为 N 个。
- Scheduler 可部署 Active/Standby，但同一时刻只有一个 Active。

### 12.2 Redis 锁

- Key：`lock:hot-rank:scheduler`。
- Value：每次持锁生成的随机 ownerToken。
- 使用 `SET NX PX` 获取。
- Active 周期续约；续约失败或锁丢失后停止调度。
- Lua 校验 ownerToken 后续约或释放。
- Active 崩溃后由锁过期触发 Standby 接管。
- TTL、续约与抢锁间隔可配置。

该实现称为“基于 Redis 分布式锁的 Scheduler 单活”，不称为自研选主算法。

### 12.3 高水位、分钟状态与有界分段

仅保存每分钟 done 标记无法枚举停机期间漏掉的分钟，因此增加无 TTL 的持久化高水位：

```text
hot:window:cursor:add
hot:window:cursor:remove
hot:window:applied:add:{minute}
hot:window:applied:remove:{minute}
hot:window:state:{minute}
hot:window:segment:{minute}
```

处理规则：

- add 只处理已经封盘的分钟，remove 只处理已经退出窗口的分钟。
- Scheduler 从对应高水位的下一分钟开始，每轮最多补跑配置数量的连续分钟。
- 空分钟同样写入对应检查点并推进高水位，避免补跑永久停在无互动分钟。
- 只有分钟完整处理后才推进对应高水位；部分分段完成后崩溃，接管者从持久化分段游标继续。
- 分钟状态至少区分 open、aggregating、added、removed。
- 开始 add 时通过 Lua 校验锁并将桶从 open 转为 aggregating，冻结原始桶。
- 大桶使用有界 ZSCAN 分段；每段在同一 Lua 中校验锁、读取有界成员、更新聚合 ZSET并保存分段游标。
- 原始桶冻结后不允许 Worker 再写入，因此不能使用会随写入变化的 rank offset 分段。
- 完成全部分段后，原子写入 add 检查点、状态和高水位。
- remove 完成后写入 remove 检查点并推进 remove 高水位。

桶、检查点和分段状态保留时间必须覆盖 60 分钟窗口、最大支持 Scheduler 停机时间和补跑余量。若桶已因超时消失而未完成 remove，必须触发基于 outcome 的明确重建，不得静默跳过。

### 12.4 迟到互动与原桶修正

已经进入 add 聚合的原始分钟桶不能再被 Worker 单独修改，否则可能出现“原桶增加但聚合榜未增加”或分段漏读。使用伴随迟到桶：

```text
hot:video:minute:{minute}
hot:video:late:{minute}
```

Worker 对 outcome 的热度效果执行原子 Lua：

1. 使用 `hot:applied:{operationId}` 去重。
2. 原桶为 open 时，只写原分钟桶，等待 Scheduler 正常聚合。
3. 原桶为 aggregating 或 added、且尚未 removed 时，同时写迟到桶和聚合 ZSET。
4. 原桶已 removed 或命令已超出窗口时，不写任何热度桶。
5. 效果标记、状态判断和对应 Redis 修改必须在同一脚本中完成。

Scheduler 的 add/remove 状态变更与 Worker 热度脚本在同一 Redis 上原子串行化。remove 必须扣减原始桶和迟到桶的完整贡献；remove 后到达的效果不再写入，避免复活已退出窗口的桶。

### 12.5 热度语义

- 热度为最近 60 分钟有效互动活动分。
- 点赞只在真实未点赞到已点赞时加分。
- 评论只在实际新增时加分。
- 取消点赞不新增活动分，原互动随原分钟桶退出。
- 重复、过期和窗口外迟到命令不污染当前热度。

### 12.6 快照分页

- 第一页创建快照并返回 `snapshotId + offset` 游标。
- 游标需要防篡改或至少严格校验格式和生命周期。
- 后续页直接按 offset 读快照，不读取实时 Top N。
- 不再通过遍历快照定位 lastVideoId。
- 页面视频使用 MGET。
- 仅缺失 ID 批量回源 MySQL并回填。
- 最终结果按快照 ID 顺序组装。
- 快照过期时明确创建新快照，不混用排序。

## 13. 配置 Spec

新增配置至少需要覆盖：

- MQ 发布 Confirm 超时、发布并发和重连策略。
- 各队列 prefetch、Worker 并发、批大小、批等待和处理超时。
- Retry/DLQ 参数与最大重试次数。
- 幂等记录 TTL。
- 数据库连接池、连接生命周期和回源并发。
- 视频缓存 TTL、抖动、空值 TTL和重建锁 TTL。
- Scheduler 锁 TTL、续约、抢锁间隔、单轮补跑分钟数和桶分段大小。
- 分钟桶、检查点、效果标记和幂等记录的保留策略。
- 热榜窗口、快照 TTL 和 Top N 上限。

所有配置需要默认值、合法性校验和 `.env.example` 文档。非法关键配置在启动时失败，不静默使用危险值。

## 14. 可观测性 Spec

至少增加结构化日志和可测试的指标钩子：

- API：Confirm 延迟/失败、限流、幂等命中、回源并发与超时。
- MQ：队列深度、最老消息年龄、重试和 DLQ。
- Worker：批大小、批耗时、实际写入行数、MySQL/Redis 重放。
- 缓存：命中率、singleflight 合并、重建锁等待和回源失败。
- Scheduler：持锁者、续约失败、锁丢失、接管、检查点命中和补跑。
- 热榜：聚合延迟、快照命中与过期。

日志使用 `operationId` 串联 API、MQ 与 Worker。不得在消息或日志中输出认证密钥。

## 15. 测试与验收 Spec

### 15.1 单元测试

- Command 编解码和未知版本。
- 操作版本生成、恢复、重复与乱序。
- 幂等键同内容重试和不同内容冲突。
- 点赞批内合并、跨批旧版本过滤和计数增量。
- 评论批量新增、重复插入、删除与计数。
- Redis Lua 重放、乱序和旧统计覆盖。
- Scheduler 锁续约、释放、接管、高低水位和分段恢复。
- open/aggregating/added/removed 各状态下的迟到热度效果。
- outcome 首处理、重放、指纹冲突和统计绝对值版本。
- 快照游标、offset 边界和结果排序。

### 15.2 集成测试

- Publisher Confirm 成功后 Accepted，失败后不接受。
- MySQL 成功、Redis 失败后的命令重放。
- Redis 成功、ACK 失败后的消息重放。
- Worker 崩溃后的重新投递。
- pending 评论删除返回 409；已提交评论的创建/删除重放与版本过滤。
- 新旧消费者滚动部署，旧消费者不会收到新 Command。
- 多 Worker 消费不会重复聚合。
- Scheduler 在分段、补跑和主备切换的各阶段退出，均不重复或遗漏分钟桶。
- 迟到互动与 add/remove 并发时，原桶、迟到桶和聚合榜保持一致。
- 缓存失效时数据库回源不超过配置上限。

### 15.3 压测场景

1. 单热点视频点赞/取消点赞混合写。
2. 单热点视频评论创建与删除。
3. 视频详情缓存失效后的并发读取。
4. 持续互动与热榜多页读取混合流量。
5. Scheduler 故障与补跑期间的持续互动。

记录环境、数据量、客户端模型、吞吐、P95/P99、错误率、MQ 最老消息年龄、MySQL 写入和 Redis 指标。没有测量前不写提升倍数或生产 QPS。

### 15.4 功能验收

1. API 只在 MQ Confirm 后返回 Accepted。
2. 同一评论命令重放不产生重复评论、计数或热度。
3. 同一用户视频键的旧操作不覆盖新状态。
4. pending 评论删除返回 409；已提交评论的旧版本操作不会覆盖新版本。
5. API 不同步逐条写入互动 MySQL。
6. Worker 正常路径不再全量读取点赞集合并执行 NOT IN。
7. 热点回源有界且可观测。
8. Scheduler 主备切换和补跑不重复聚合。
9. 热榜后续分页不访问实时榜单，页面顺序与快照一致。
10. 前端 pending 和重试语义与后端一致。

## 16. 实施拆分与滚动迁移

### Milestone A：Expand schema 与领域模型

- 添加 likes/comments 版本、`video_stats` 和 `interaction_outcomes` 迁移。
- 评论 ID 改为 UUID 字符串，提供旧数据转换和回滚备份。
- 添加 Command 模型、Redis 幂等记录和集中版本生成器。
- 改造 MQ 拓扑和复用发布器，但新 API 暂不开放。

### Milestone B：先部署兼容消费者

- 新旧 Event/Command 使用独立 Exchange/Queue，禁止混投。
- 部署同时支持旧 Event 与新 Command 的 Worker，新消费者就绪前不得启用新 API。
- 实现 prefetch、Worker Pool、批次生命周期和 ACK 边界。
- 完成点赞条件 UPSERT、评论按视频批量写入、video_stats 和 outcome 重放。
- 验证旧消费者继续处理旧队列，新消费者只处理新队列。

### Milestone C：开放新 API 与前端适配

- 前端先发布 Idempotency-Key 兼容改造，旧后端可忽略该头。
- 确认新消费者就绪后，再开放新 API 和 202/pending 响应。
- 点赞、评论只写新队列，不对同一逻辑操作双写新旧队列。
- 新 API 已接受命令后不能直接回滚到旧 API；回滚前必须停止新发布并排空或继续消费新命令。

### Milestone D：热点读与 Scheduler

- 拆分缓存、统计回源、singleflight、重建锁和有界回源。
- 新增 `cmd/scheduler`，移除 Worker 定时窗口职责。
- 实现单活锁、续约、高低水位、有界分段、迟到桶和补跑。
- 实现快照 offset 游标与 MGET 页面组装。

### Milestone E：停止旧写路径、核对并 Contract

- 停止旧 API publisher 和旧脏集合 writer。
- 排空旧队列，再执行最终 stats、Redis 状态和热榜核对。
- 核对完成后切换读接口到 video_stats，并移除旧全量同步主路径。
- 完成单元、集成、故障注入和压测，记录环境与结果。
- 最后移除旧协议和临时兼容代码，更新文档。

关键顺序为：

```text
expand schema/backfill
→ 部署新旧格式兼容消费者（独立队列）
→ 前端幂等键兼容
→ 开放新 API
→ 停止旧 writer 并排干旧队列
→ 最终对账
→ 切换读模型与 contract
```

每个 Milestone 保持可编译；滚动回滚必须遵守命令已接受后的排空约束，不能简单切回旧 writer。

## 17. 已收敛的实现决策

1. **ID**：operationId/commentId 使用 UUID 字符串；评论数据库、Go 和前端统一迁移为字符串。
2. **操作版本**：使用 Redis 按状态业务键集中 INCR，MySQL 保存最后提交版本；不使用 Snowflake 作为版本。
3. **幂等重试**：Redis 幂等记录保存完整不可变 Command 和消息字节，Confirm-unknown 只重发同一 Command。
4. **pending 删除**：首版返回 409 `comment_not_ready`，不实现 delete-before-create tombstone。
5. **重放依据**：强制事务性 interaction_outcomes；Redis 统计采用绝对值 + stats_version，热度按 operationId 去重。
6. **Scheduler**：单活锁之外增加 add/remove 高水位、分钟状态、有界分段和迟到桶修正。
7. **滚动迁移**：兼容消费者先上线，新 API 后开放；新旧队列隔离，最终核对在旧 writer 停止之后执行。
8. **故障取舍**：Redis 元数据全量丢失时写链路 fail-closed，不牺牲顺序换取静默可用性。

## 18. Bounded Council 评审记录

### 18.1 问题与范围

评审问题是：当前 Draft 能否直接指导编码，以及如何补齐会破坏幂等、顺序、统计、热榜和滚动迁移正确性的空缺。范围仅限本 Spec 和当前 Go 代码，不进行实现或一般性架构扩展。

### 18.2 Roster 与轮次

- 可用 profile 中没有 `council-*`，按 council 规则使用 fallback `oracle` 和 `reviewer`。
- `oracle` 使用 `context=fork`；`reviewer` 使用正常 profile，请求时上下文模式为 runtime-default-unknown。
- 第 1 轮为独立评审，第 2 轮为交叉评审；共 2 轮，未达到 3 轮上限即收敛。
- 首次 Workflow `dbcb6b98-36f2-4e88-ae31-e4f8bdf241e3` 因输出 Schema 缺少枚举字段类型而失败，未产生评审，不计入有效轮次。

### 18.3 有效证据与运行记录

- Pass 1 Workflow：`038ae519-e87b-4cc3-8790-a194f41cbfa3`。
- Pass 1 oracle：`1091128f-9bed-450e-b6a3-7f6e95907753`。
- Pass 1 reviewer：`56457cd6-aa3f-4856-b298-cdf1f362b38a`。
- Pass 2 Workflow：`2e4c1b9f-d5c8-402f-a1cd-109d3614c47e`。
- Pass 2 oracle：`4015aeb0-298d-4bb5-9ffd-dc129277b9aa`。
- Pass 2 reviewer：`672ccec3-1787-4e34-a0e4-c75a81e1268d`。
- 主要代码证据：当前 MQ 发布器、Worker 消费/ACK、点赞 Redis 多步更新、评论模型与 Handler、likes/comments migrations、热榜窗口实现。

### 18.4 采纳的反馈

- 不可变 Command 解决 Confirm-unknown 重试身份问题。
- 集中序列代替时钟相关的 Snowflake 操作版本。
- pending 删除首版返回 409，降低 tombstone 证明范围。
- MySQL outcome 作为 Redis 重放依据，避免数据库 no-op 被误认为 Redis 已完成。
- 统计使用绝对值和版本，热度使用独立效果去重。
- Scheduler 增加高水位、分段恢复、迟到桶和保留/重建规则。
- 新旧队列隔离，兼容消费者先于新 API 部署。

### 18.5 拒绝或延后的方案

- 拒绝 Snowflake 操作版本：跨节点时钟偏斜不能证明因果顺序。
- 拒绝首版 pending 删除 tombstone：证明成本高，不服务于本期热点主线。
- 拒绝新 API 先于兼容消费者上线：旧消费者会 ACK 未知类型并丢命令。
- 拒绝无界全桶 Lua 和可变 rank offset 分段。
- 延后 outcome 清理、Redis Cluster、大规模点赞分片和完整分布式选主。

### 18.6 Owner decision

两位顾问在评论 ID 上分别建议 UUID 字符串与 Snowflake 数值转字符串。主 agent 选择 UUID 字符串：实现更少节点/时钟假设，且本期允许同步修改数据库、Go 和前端。两位顾问对最小效果表结构略有差异，主 agent 选择统一 interaction_outcomes，以单一重放协议降低首版实现分支；首版不清理该表。

### 18.7 置信度与改变条件

对 Spec 的故障语义和实现前置条件置信度高，因为阻塞点均有当前代码或明确算法支撑；对性能收益尚无定量结论，必须由压测验证。

如果后续要求允许 pending 删除、不能接受 UUID 迁移、Redis 元数据可用性假设不成立，或大桶分段实测仍有阻塞，则必须重新评审对应部分，而不是在编码中静默改变语义。
