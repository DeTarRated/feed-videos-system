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

### 响应格式

| 类型 | 示例 |
| --- | --- |
| 成功 | `{"data": {...}}` |
| 失败 | `{"error": "error message"}` |

### 登录鉴权

需要登录的接口统一使用：

```text
Authorization: Bearer {{token}}
```

---

## 1. 健康检查

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `http://localhost:8080/healthz` |
| 认证 | 无 |

### 返回示例

```json
{
  "status": "ok",
  "timestamp": "2026-07-28T00:00:00Z"
}
```

---

## 2. 账号接口

### 2.1 注册

| 项 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/auth/register` |
| 认证 | 无 |
| Content-Type | `application/json` |

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `username` | string | 是 | 用户名 |
| `password` | string | 是 | 密码，至少 6 位 |
| `nickname` | string | 否 | 昵称，不传则默认使用用户名 |

#### 请求示例

```json
{
  "username": "demo",
  "password": "123456",
  "nickname": "Demo"
}
```

#### 返回示例

```json
{
  "data": {
    "token": "eyJhbGciOi...",
    "user": {
      "id": 1,
      "username": "demo",
      "nickname": "Demo",
      "avatarUrl": "",
      "bio": ""
    }
  }
}
```

#### Apifox 检查点

| 项 | 说明 |
| --- | --- |
| `data.token` | 保存到环境变量 `token` |
| `data.user.id` | 非空 |
| `data.user.username` | 等于请求用户名 |

---

### 2.2 登录

| 项 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/auth/login` |
| 认证 | 无 |
| Content-Type | `application/json` |

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `username` | string | 是 | 用户名 |
| `password` | string | 是 | 密码 |

#### 请求示例

```json
{
  "username": "demo",
  "password": "123456"
}
```

#### 返回示例

```json
{
  "data": {
    "token": "eyJhbGciOi...",
    "user": {
      "id": 1,
      "username": "demo",
      "nickname": "Demo",
      "avatarUrl": "",
      "bio": ""
    }
  }
}
```

#### Apifox 检查点

| 项 | 说明 |
| --- | --- |
| `data.token` | 保存到环境变量 `token` |
| `data.user.username` | 等于 `demo` |

---

## 3. 上传接口

上传接口都需要登录。返回的 `url` 是相对路径，前端或 Apifox 访问静态资源时拼接 `mediaBaseUrl`。

### 3.1 上传视频文件

| 项 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/uploads/videos` |
| 认证 | `Authorization: Bearer {{token}}` |
| Body 类型 | `multipart/form-data` |
| 文件字段 | `file` |

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `file` | file | 是 | 视频文件，支持 `.mp4 .mov .webm .mkv .m4v` |

#### 请求示例

| 字段 | 值 |
| --- | --- |
| `file` | 选择一个视频文件 |

#### 返回示例

```json
{
  "data": {
    "url": "/media/videos/20260728/1710000000000000000.mp4",
    "name": "1710000000000000000.mp4",
    "size": 12345678
  }
}
```

#### Apifox 检查点

| 项 | 说明 |
| --- | --- |
| `data.url` | 保存给后续发布视频使用 |
| `data.url` 访问 | 组合 `{{mediaBaseUrl}}` 后应能直接访问 |

---

### 3.2 上传封面文件

| 项 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/uploads/covers` |
| 认证 | `Authorization: Bearer {{token}}` |
| Body 类型 | `multipart/form-data` |
| 文件字段 | `file` |

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `file` | file | 是 | 图片文件，支持 `.jpg .jpeg .png .webp .gif` |

#### 请求示例

| 字段 | 值 |
| --- | --- |
| `file` | 选择一个图片文件 |

#### 返回示例

```json
{
  "data": {
    "url": "/media/covers/20260728/1710000000000000001.jpg",
    "name": "1710000000000000001.jpg",
    "size": 123456
  }
}
```

#### Apifox 检查点

| 项 | 说明 |
| --- | --- |
| `data.url` | 保存给后续发布视频使用 |
| `data.url` 访问 | 组合 `{{mediaBaseUrl}}` 后应能直接访问 |

---

## 4. 点赞接口

### 4.1 点赞视频

| 项 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/videos/:id/like` |
| 认证 | `Authorization: Bearer {{token}}` |

#### 路径参数

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | int | 是 | 视频 ID |

#### 返回示例

```json
{
  "data": {
    "videoId": 1,
    "liked": true,
    "likeCount": 1
  }
}
```

### 4.2 取消点赞

| 项 | 内容 |
| --- | --- |
| 方法 | `DELETE` |
| URL | `{{baseUrl}}/videos/:id/like` |
| 认证 | `Authorization: Bearer {{token}}` |

#### 返回示例

```json
{
  "data": {
    "videoId": 1,
    "liked": false,
    "likeCount": 0
  }
}
```

### 4.3 查询点赞状态

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/videos/:id/like-status` |
| 认证 | 可选，不登录也可查点赞数 |

#### 返回示例

```json
{
  "data": {
    "videoId": 1,
    "liked": true,
    "likeCount": 3
  }
}
```

### 4.4 我的点赞视频列表

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/users/me/liked-videos` |
| 认证 | `Authorization: Bearer {{token}}` |

#### Query 参数

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `limit` | int | 否 | `20` | 返回数量上限 |

#### 返回示例

```json
{
  "data": {
    "items": [
      {
        "id": 1,
        "authorId": 1,
        "title": "第一个视频",
        "description": "MVP 上传发布测试",
        "videoUrl": "/media/videos/20260728/1710000000000000000.mp4",
        "coverUrl": "/media/covers/20260728/1710000000000000001.jpg",
        "publishedAt": "2026-07-28T00:00:00Z",
        "createdAt": "2026-07-28T00:00:00Z",
        "updatedAt": "2026-07-28T00:00:00Z"
      }
    ]
  }
}
```

---

## 5. 视频接口

### 5.1 发布视频

| 项 | 内容 |
| --- | --- |
| 方法 | `POST` |
| URL | `{{baseUrl}}/videos` |
| 认证 | `Authorization: Bearer {{token}}` |
| Content-Type | `application/json` |

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `title` | string | 是 | 视频标题 |
| `description` | string | 否 | 视频描述 |
| `videoUrl` | string | 是 | 上传视频返回的 `url` |
| `coverUrl` | string | 否 | 上传封面返回的 `url` |

#### 请求示例

```json
{
  "title": "第一个视频",
  "description": "MVP 上传发布测试",
  "videoUrl": "/media/videos/20260728/1710000000000000000.mp4",
  "coverUrl": "/media/covers/20260728/1710000000000000001.jpg"
}
```

#### 返回示例

```json
{
  "data": {
    "id": 1,
    "authorId": 1,
    "title": "第一个视频",
    "description": "MVP 上传发布测试",
    "videoUrl": "/media/videos/20260728/1710000000000000000.mp4",
    "coverUrl": "/media/covers/20260728/1710000000000000001.jpg",
    "publishedAt": "2026-07-28T00:00:00Z",
    "createdAt": "2026-07-28T00:00:00Z",
    "updatedAt": "2026-07-28T00:00:00Z"
  }
}
```

### 5.2 最新视频流

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/videos` |
| 认证 | 无 |

#### Query 参数

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `limit` | int | 否 | `20` | 返回数量上限 |

### 5.3 视频详情

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/videos/:id` |
| 认证 | 无 |

---

## 6. 用户接口

### 6.1 用户主页统计

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/users/:id/stats` |
| 认证 | 无 |

### 6.2 作者作品列表

| 项 | 内容 |
| --- | --- |
| 方法 | `GET` |
| URL | `{{baseUrl}}/users/:id/videos` |
| 认证 | 无 |

---

## 7. 静态资源访问

上传接口返回的 `/media/...` URL 由后端静态资源服务提供。

| 项 | 内容 |
| --- | --- |
| 访问地址示例 | `http://localhost:8080/media/videos/20260728/1710000000000000000.mp4` |
| 验证方式 | 在浏览器或 Apifox 中直接打开该地址 |
| 预期结果 | 返回对应文件内容 |
