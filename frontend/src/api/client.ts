const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1'
const MEDIA_BASE_URL = API_BASE_URL.replace(/\/api\/v1\/?$/, '')

export type ApiResponse<T> = {
  data?: T
  error?: string
}

export type User = {
  id: number
  username: string
  nickname: string
  avatarUrl: string
  bio: string
}

export type AuthResult = {
  token: string
  user: User
}

export type Video = {
  id: number
  authorId: number
  title: string
  description: string
  videoUrl: string
  coverUrl: string
  publishedAt: string
  createdAt: string
  updatedAt: string
  commentCount?: number
}

export type UploadResult = {
  url: string
  name: string
  size: number
}

export type UserStats = {
  userId: number
  worksCount: number
  likeCount: number
  followerCount: number
  followingCount: number
}

export type LikeStatus = {
  videoId: number
  liked: boolean
  likeCount: number
}

export type Comment = {
  id: number
  userId: number
  userNickname: string
  videoId: number
  content: string
  createdAt: string
  updatedAt: string
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
    ...options,
  })
  const body = (await response.json()) as ApiResponse<T>
  if (!response.ok) {
    throw new Error(body.error || 'request failed')
  }
  return body.data as T
}

async function upload(path: string, token: string, file: File): Promise<UploadResult> {
  const formData = new FormData()
  formData.append('file', file)

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: formData,
  })
  const body = (await response.json()) as ApiResponse<UploadResult>
  if (!response.ok) {
    throw new Error(body.error || 'upload failed')
  }
  return body.data as UploadResult
}

export function resolveMediaUrl(path: string) {
  if (!path) {
    return ''
  }
  if (/^https?:\/\//.test(path)) {
    return path
  }
  return `${MEDIA_BASE_URL}${path.startsWith('/') ? path : `/${path}`}`
}

export function register(payload: { username: string; password: string; nickname?: string }) {
  return request<AuthResult>('/auth/register', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function login(payload: { username: string; password: string }) {
  return request<AuthResult>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function listVideos() {
  return request<{ items: Video[] }>('/videos')
}

export function getVideo(id: string) {
  return request<Video>(`/videos/${id}`)
}

export function getVideoLikeStatus(id: string, token?: string) {
  return request<LikeStatus>(`/videos/${id}/like-status`, token ? {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  } : undefined)
}

export function likeVideo(token: string, id: string) {
  return request<LikeStatus>(`/videos/${id}/like`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function unlikeVideo(token: string, id: string) {
  return request<LikeStatus>(`/videos/${id}/like`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function listComments(id: string, limit = 50) {
  return request<{ items: Comment[] }>(`/videos/${id}/comments?limit=${limit}`)
}

export function createComment(token: string, id: string, content: string) {
  return request<Comment>(`/videos/${id}/comments`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ content }),
  })
}

export function deleteComment(token: string, videoId: string, commentId: number) {
  return request<{ deleted: boolean }>(`/videos/${videoId}/comments/${commentId}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function getUserStats(id: string) {
  return request<UserStats>(`/users/${id}/stats`)
}

export function listUserVideos(id: string, limit = 20) {
  return request<{ items: Video[] }>(`/users/${id}/videos?limit=${limit}`)
}

export function listLikedVideos(token: string, limit = 20) {
  return request<{ items: Video[] }>(`/users/me/liked-videos?limit=${limit}`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export function uploadVideo(token: string, file: File) {
  return upload('/uploads/videos', token, file)
}

export function uploadCover(token: string, file: File) {
  return upload('/uploads/covers', token, file)
}

export function publishVideo(token: string, payload: { title: string; description?: string; videoUrl: string; coverUrl?: string }) {
  return request<Video>('/videos', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  })
}
