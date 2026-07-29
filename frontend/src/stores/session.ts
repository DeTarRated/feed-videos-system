import { defineStore } from 'pinia'
import type { User } from '../api/client'

function readStoredUser() {
  const raw = localStorage.getItem('user')
  if (!raw) {
    return null
  }
  try {
    return JSON.parse(raw) as User
  } catch {
    localStorage.removeItem('user')
    return null
  }
}

export const useSessionStore = defineStore('session', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    user: readStoredUser(),
  }),
  getters: {
    isLoggedIn: (state) => state.token !== '',
  },
  actions: {
    setSession(token: string, user: User) {
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
    },
    clearSession() {
      this.token = ''
      this.user = null
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    },
  },
})
