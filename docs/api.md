# API 文档

## 通用约定

### Base URL

```text
http://localhost:8080/api/v1
```

### Apifox 环境变量

| 变量名 | 示例值 | 说明 |
| --- | --- | --- |
| `baseUrl` | `http://localhost:8080/api/v1` | API 基础地址 |
| `token` | 空 | 登录后保存 JWT |
| `mediaBaseUrl` | `http://localhost:8080` | 静态媒体访问根地址 |

### 登录鉴权

```text
Authorization: Bearer {{token}}
```

成功响应统一为 `{"data": ...}`，失败响应统一为 `{"error": "..."}`。

## 账号与视频基础接口

| 方法 | URL | 认证 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/healthz` | 无 | 健康检查 |
| `POST` | `/auth/register` | 无 | 注册 |
| `POST` | `/auth/login` | 无 | 登录并获取 Token |
| `GET` | `/videos` | 无 | 最新视频流 |
| `GET` | `/videos/:id` | 无 | 视频详情 |
| `POST` | `/videos` | 必须 | 发布视频 |
| `POST` | `/uploads/videos` | 必须 | 上传视频 |
| `POST` | `/uploads/covers` | 必须 | 上传封面 |
| `GET` | `/users/:id/stats` | 无 | 用户主页统计 |
| `GET` | `/users/:id/videos` | 无 | 作者作品列表 |

### 视频详情响应

视频详情响应在基础视频字段之外返回当前未删除评论总数：

```json
{
  "data": {
    "id": 1,
    "authorId": 2,
    "title": "示例视频",
    "commentCount": 12
  }
}
```

## 点赞接口

| 方法 | URL | 认证 | 说明 |
| --- | --- | --- | --- |
| `POST` | `{{baseUrl}}/videos/:id/like` | 必须 | 点赞视频，重复调用幂等 |
| `DELETE` | `{{baseUrl}}/videos/:id/like` | 必须 | 取消点赞，重复调用幂等 |
| `GET` | `{{baseUrl}}/videos/:id/like-status` | 可选 | 查询当前用户点赞状态和总数 |
| `GET` | `{{baseUrl}}/users/me/liked-videos` | 必须 | 我的点赞视频列表 |

### 点赞/取消点赞请求

路径参数：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | int | 是 | 视频 ID |

点赞和取消点赞无请求 Body。

返回示例：

```json
{
  "data": {
    "videoId": 1,
    "liked": true,
    "likeCount": 1
  }
}
```

### 点赞状态

登录用户查询时必须带：

```text
Authorization: Bearer {{token}}
```

匿名查询也可以执行，但 `liked` 固定为 `false`，只能查看公共点赞数。

## 评论接口

| 方法 | URL | 认证 | Body | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `{{baseUrl}}/videos/:id/comments` | 无 | 无 | 评论列表 |
| `POST` | `{{baseUrl}}/videos/:id/comments` | 必须 | JSON | 发表评论 |
| `DELETE` | `{{baseUrl}}/videos/:id/comments/:commentId` | 必须 | 无 | 删除自己的评论 |

### 评论列表

Query 参数：

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `limit` | int | 否 | `50` | 最大 100 |

返回示例：

```json
{
  "data": {
    "items": [
      {
        "id": 1,
        "userId": 2,
        "userNickname": "小明",
        "videoId": 1,
        "content": "很不错的视频",
        "createdAt": "2026-07-28T00:00:00Z",
        "updatedAt": "2026-07-28T00:00:00Z"
      }
    ]
  }
}
```

### 发表评论

Headers：

```text
Content-Type: application/json
Authorization: Bearer {{token}}
```

Body JSON：

```json
{
  "content": "很不错的视频"
}
```

规则：

- `content` 必填。
- 去除首尾空白后不能为空。
- 最大 500 个字符。

### 删除评论

Headers：

```text
Authorization: Bearer {{token}}
```

只能删除当前登录用户自己发表的评论。成功返回：

```json
{
  "data": {
    "deleted": true
  }
}
```

## 用户接口

| 方法 | URL | 认证 | 说明 |
| --- | --- | --- | --- |
| `GET` | `{{baseUrl}}/users/:id/stats` | 无 | 返回作品数、获赞数、粉丝数、关注数 |
| `GET` | `{{baseUrl}}/users/:id/videos` | 无 | 返回作者作品 |

当前用户统计中的 `likeCount` 已接入真实点赞数据，关注相关字段将在关注模块完成后接入。

## 静态资源

```text
http://localhost:8080/media/videos/20260728/example.mp4
```
