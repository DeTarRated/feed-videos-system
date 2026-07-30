<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  changePassword,
  followUser,
  getCurrentUser,
  getFollowStatus,
  getUser,
  getUserStats,
  listFollowers,
  listFollowing,
  listLikedVideos,
  listUserVideos,
  resolveMediaUrl,
  unfollowUser,
  updateProfile,
  type FollowStatus,
  type FollowUser,
  type User,
  type UserStats,
  type Video,
} from '../api/client'
import { useSessionStore } from '../stores/session'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const profile = ref<User | null>(null)
const stats = ref<UserStats | null>(null)
const followStatus = ref<FollowStatus | null>(null)
const videos = ref<Video[]>([])
const likedVideos = ref<Video[]>([])
const followers = ref<FollowUser[]>([])
const following = ref<FollowUser[]>([])
const activeTab = ref<'works' | 'liked' | 'publish'>('works')
const relationView = ref<'followers' | 'following' | null>(null)
const settingsOpen = ref(false)
const settingsTab = ref<'account' | 'privacy' | 'logout'>('account')
const likedPublic = ref(false)
const loading = ref(false)
const actionLoading = ref(false)
const savingProfile = ref(false)
const changingPassword = ref(false)
const error = ref('')
const profileMessage = ref('')
const passwordMessage = ref('')
const editOpen = ref(false)
const passwordOpen = ref(false)
const profileForm = ref({ nickname: '', avatarUrl: '', bio: '' })
const passwordForm = ref({ oldPassword: '', newPassword: '' })

const profileUserId = computed(() => Number(route.params.id))
const isSelfProfile = computed(() => session.user?.id === profileUserId.value)
const followButtonLabel = computed(() => followStatus.value?.following ? '取消关注' : '关注')

async function load() {
  loading.value = true
  error.value = ''
  profileMessage.value = ''
  passwordMessage.value = ''
  try {
    const userId = String(route.params.id)
    if (session.token) {
      try {
        const current = await getCurrentUser(session.token)
        session.setUser(current)
      } catch {
        session.clearSession()
      }
    }
    const [profileResult, statsResult, videosResult, statusResult, followersResult, followingResult] = await Promise.all([
      getUser(userId),
      getUserStats(userId),
      listUserVideos(userId),
      getFollowStatus(userId, session.token),
      listFollowers(userId),
      listFollowing(userId),
    ])
    profile.value = profileResult
    stats.value = statsResult
    followStatus.value = statusResult
    videos.value = videosResult.items || []
    followers.value = followersResult.items || []
    following.value = followingResult.items || []
    profileForm.value = {
      nickname: profileResult.nickname,
      avatarUrl: profileResult.avatarUrl,
      bio: profileResult.bio,
    }
    likedVideos.value = []
    if (isSelfProfile.value && session.token) {
      const likedResult = await listLikedVideos(session.token)
      likedVideos.value = likedResult.items || []
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function toggleFollow() {
  if (!session.token || isSelfProfile.value) {
    return
  }
  actionLoading.value = true
  error.value = ''
  try {
    const userId = String(route.params.id)
    const next = followStatus.value?.following
      ? await unfollowUser(session.token, userId)
      : await followUser(session.token, userId)
    followStatus.value = next
    if (stats.value) {
      stats.value.followerCount = next.followerCount
    }
    const followersResult = await listFollowers(userId)
    followers.value = followersResult.items || []
  } catch (err) {
    error.value = err instanceof Error ? err.message : '操作失败'
  } finally {
    actionLoading.value = false
  }
}

async function saveProfile() {
  if (!session.token) {
    return
  }
  savingProfile.value = true
  error.value = ''
  profileMessage.value = ''
  try {
    const updated = await updateProfile(session.token, profileForm.value)
    profile.value = updated
    session.setUser(updated)
    profileMessage.value = '资料已更新'
    editOpen.value = false
  } catch (err) {
    error.value = err instanceof Error ? err.message : '保存失败'
  } finally {
    savingProfile.value = false
  }
}

async function savePassword() {
  if (!session.token) {
    return
  }
  changingPassword.value = true
  error.value = ''
  passwordMessage.value = ''
  try {
    await changePassword(session.token, passwordForm.value)
    passwordForm.value = { oldPassword: '', newPassword: '' }
    passwordMessage.value = '密码已更新'
    passwordOpen.value = false
  } catch (err) {
    error.value = err instanceof Error ? err.message : '修改失败'
  } finally {
    changingPassword.value = false
  }
}

function logout() {
  session.clearSession()
  settingsOpen.value = false
  router.push('/login')
}

function openRelation(view: 'followers' | 'following') {
  relationView.value = view
}

function openSettings(tab: 'account' | 'privacy' | 'logout' = 'account') {
  settingsTab.value = tab
  settingsOpen.value = true
}

onMounted(load)
watch(() => route.params.id, () => {
  activeTab.value = 'works'
  relationView.value = null
  editOpen.value = false
  passwordOpen.value = false
  settingsOpen.value = false
  settingsTab.value = 'account'
  load()
})
</script>

<template>
  <section class="page profile-page">
    <p v-if="loading">加载中...</p>
    <p v-else-if="error && !profile" class="error">{{ error }}</p>

    <template v-else-if="profile">
      <header class="profile-header">
        <img v-if="profile.avatarUrl" class="profile-avatar" :src="resolveMediaUrl(profile.avatarUrl)" :alt="profile.nickname" />
        <div v-else class="profile-avatar profile-avatar-placeholder">{{ profile.nickname.slice(0, 1) }}</div>
        <div class="profile-summary">
          <h1>{{ profile.nickname }}</h1>
          <p>@{{ profile.username }}</p>
          <p>{{ profile.bio || '这个用户还没有填写简介。' }}</p>
          <dl v-if="stats" class="profile-stats">
            <div><dt>作品</dt><dd>{{ stats.worksCount }}</dd></div>
            <div><dt>获赞</dt><dd>{{ stats.likeCount }}</dd></div>
            <button type="button" @click="openRelation('followers')"><dt>粉丝</dt><dd>{{ stats.followerCount }}</dd></button>
            <button type="button" @click="openRelation('following')"><dt>关注</dt><dd>{{ stats.followingCount }}</dd></button>
          </dl>
        </div>
        <div class="profile-actions">
          <button
            v-if="session.token && !isSelfProfile"
            type="button"
            :disabled="actionLoading"
            @click="toggleFollow"
          >
            {{ actionLoading ? '处理中...' : followButtonLabel }}
          </button>
          <template v-if="isSelfProfile">
            <button type="button" @click="editOpen = !editOpen">编辑资料</button>
            <button class="icon-button" type="button" aria-label="设置" title="设置" @click="openSettings()">⚙</button>
          </template>
        </div>
      </header>

      <form v-if="editOpen" class="profile-form" @submit.prevent="saveProfile">
        <label>
          昵称
          <input v-model="profileForm.nickname" required maxlength="64" />
        </label>
        <label>
          头像地址
          <input v-model="profileForm.avatarUrl" maxlength="255" />
        </label>
        <label>
          简介
          <textarea v-model="profileForm.bio" rows="3" maxlength="255" />
        </label>
        <button type="submit" :disabled="savingProfile || !profileForm.nickname.trim()">
          {{ savingProfile ? '保存中...' : '保存资料' }}
        </button>
      </form>

      <div v-if="settingsOpen" class="settings-modal" role="dialog" aria-modal="true" aria-label="设置">
        <div class="settings-panel">
          <aside class="settings-menu">
            <button type="button" :class="{ active: settingsTab === 'account' }" @click="settingsTab = 'account'">管理账号</button>
            <button type="button" :class="{ active: settingsTab === 'privacy' }" @click="settingsTab = 'privacy'">隐私</button>
            <button type="button" :class="{ active: settingsTab === 'logout' }" @click="settingsTab = 'logout'">退出登录</button>
          </aside>
          <section class="settings-content">
            <div class="section-title">
              <h2>{{ settingsTab === 'account' ? '管理账号' : settingsTab === 'privacy' ? '隐私' : '退出登录' }}</h2>
              <button class="link-button" type="button" @click="settingsOpen = false">关闭</button>
            </div>
            <form v-if="settingsTab === 'account'" class="profile-form" @submit.prevent="savePassword">
              <label>
                当前密码
                <input v-model="passwordForm.oldPassword" type="password" required minlength="6" />
              </label>
              <label>
                新密码
                <input v-model="passwordForm.newPassword" type="password" required minlength="6" />
              </label>
              <button type="submit" :disabled="changingPassword || !passwordForm.oldPassword || !passwordForm.newPassword">
                {{ changingPassword ? '修改中...' : '修改密码' }}
              </button>
            </form>
            <div v-else-if="settingsTab === 'privacy'" class="privacy-options">
              <span>喜欢的作品是否公开</span>
              <div>
                <button type="button" :class="{ active: likedPublic }" @click="likedPublic = true">是</button>
                <button type="button" :class="{ active: !likedPublic }" @click="likedPublic = false">否</button>
              </div>
            </div>
            <div v-else class="logout-confirm">
              <p>是否确认退出登录？</p>
              <button type="button" @click="logout">是</button>
            </div>
          </section>
        </div>
      </div>

      <p v-if="profileMessage" class="success">{{ profileMessage }}</p>
      <p v-if="passwordMessage" class="success">{{ passwordMessage }}</p>
      <p v-if="error" class="error">{{ error }}</p>

      <nav class="profile-tabs" aria-label="用户主页内容">
        <button type="button" :class="{ active: activeTab === 'works' }" @click="activeTab = 'works'">作品</button>
        <button type="button" :class="{ active: activeTab === 'liked' }" @click="activeTab = 'liked'">喜欢</button>
        <RouterLink class="profile-tab-link" to="/publish">发布</RouterLink>
      </nav>

      <section v-if="relationView" class="profile-section">
        <div class="section-title">
          <h2>{{ relationView === 'followers' ? '粉丝' : '关注' }}</h2>
          <button class="link-button" type="button" @click="relationView = null">返回内容</button>
        </div>
        <p v-if="relationView === 'followers' && followers.length === 0">暂无粉丝。</p>
        <p v-else-if="relationView === 'following' && following.length === 0">暂无关注。</p>
        <ul v-else class="user-list">
          <li v-for="user in relationView === 'followers' ? followers : following" :key="user.id">
            <RouterLink :to="`/users/${user.id}`">{{ user.nickname || user.username }}</RouterLink>
            <span>{{ user.bio || user.username }}</span>
          </li>
        </ul>
      </section>

      <section v-else-if="activeTab === 'works'" class="profile-section">
        <p v-if="videos.length === 0">暂无作品。</p>
        <ul v-else class="video-list">
          <li v-for="video in videos" :key="video.id">
            <RouterLink :to="`/video/${video.id}`">{{ video.title }}</RouterLink>
            <img v-if="video.coverUrl" :src="resolveMediaUrl(video.coverUrl)" :alt="video.title" />
            <span>{{ video.description || '无描述' }}</span>
          </li>
        </ul>
      </section>

      <section v-else-if="activeTab === 'liked'" class="profile-section">
        <p v-if="!isSelfProfile">喜欢列表暂时仅本人可见。</p>
        <p v-else-if="likedVideos.length === 0">暂无喜欢的视频。</p>
        <ul v-else class="video-list">
          <li v-for="video in likedVideos" :key="video.id">
            <RouterLink :to="`/video/${video.id}`">{{ video.title }}</RouterLink>
            <img v-if="video.coverUrl" :src="resolveMediaUrl(video.coverUrl)" :alt="video.title" />
            <span>{{ video.description || '无描述' }}</span>
          </li>
        </ul>
      </section>

      <section v-else class="profile-section">
        <p>从上方选择要查看的内容。</p>
      </section>
    </template>
  </section>
</template>
