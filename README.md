# Feed Videos System

一个从零实现的短视频 Feed 系统面试项目。

## 目标

- 先完成项目骨架和文档设计
- 再逐步实现后端、前端、缓存、异步任务和通知链路
- 最终形成可演示、可部署、可讲解的完整项目

## 目录规划

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
    cmd/api/
    internal/
    migrations/
  frontend/
    src/
```

## 当前阶段

当前进入第一阶段 MVP 基础版。已完成文档初始化，并开始搭建后端 Go 服务骨架和前端 Vue 入口。

## 本地开发

### 后端

```bash
cd backend
go mod tidy
go run ./cmd/api
```

健康检查：

```bash
curl http://localhost:8080/healthz
```

### 前端

```bash
cd frontend
npm install
npm run dev
```

默认前端地址：`http://localhost:5173`。

### 配置

复制 `.env.example` 为 `.env` 后按本地环境调整。MVP 阶段默认使用本地 MySQL 和本地文件存储。
