<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { publishVideo, uploadCover, uploadVideo } from '../api/client'
import { useSessionStore } from '../stores/session'

const router = useRouter()
const session = useSessionStore()
const title = ref('')
const description = ref('')
const videoFile = ref<File | null>(null)
const coverFile = ref<File | null>(null)
const error = ref('')
const loading = ref(false)
const status = ref('')

function onVideoChange(event: Event) {
  const input = event.target as HTMLInputElement
  videoFile.value = input.files?.[0] || null
}

function onCoverChange(event: Event) {
  const input = event.target as HTMLInputElement
  coverFile.value = input.files?.[0] || null
}

async function submit() {
  if (!session.token) {
    router.push('/login')
    return
  }
  if (!videoFile.value) {
    error.value = '请选择视频文件'
    return
  }

  loading.value = true
  error.value = ''
  status.value = '上传视频中...'
  try {
    const uploadedVideo = await uploadVideo(session.token, videoFile.value)
    let coverUrl = ''
    if (coverFile.value) {
      status.value = '上传封面中...'
      const uploadedCover = await uploadCover(session.token, coverFile.value)
      coverUrl = uploadedCover.url
    }

    status.value = '发布视频中...'
    const video = await publishVideo(session.token, {
      title: title.value,
      description: description.value,
      videoUrl: uploadedVideo.url,
      coverUrl,
    })
    router.push(`/video/${video.id}`)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '发布失败'
  } finally {
    loading.value = false
    status.value = ''
  }
}
</script>

<template>
  <section class="page narrow">
    <h1>发布视频</h1>
    <form class="form" @submit.prevent="submit">
      <label>
        标题
        <input v-model="title" required maxlength="128" />
      </label>
      <label>
        视频文件
        <input type="file" accept="video/*" required @change="onVideoChange" />
      </label>
      <label>
        封面文件
        <input type="file" accept="image/*" @change="onCoverChange" />
      </label>
      <label>
        描述
        <textarea v-model="description" rows="4" maxlength="500" />
      </label>
      <p v-if="status">{{ status }}</p>
      <p v-if="error" class="error">{{ error }}</p>
      <button type="submit" :disabled="loading">{{ loading ? '处理中...' : '发布' }}</button>
    </form>
  </section>
</template>
