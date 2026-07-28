import { defineStore } from 'pinia'
import type { User } from '../api/client'

export const useSessionStore = defineStore('session', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    user: null as User | null,
  }),
  getters: {
    isLoggedIn: (state) => state.token !== '',
  },
  actions: {
    setSession(token: string, user: User) {
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
    },
    clearSession() {
      this.token = ''
      this.user = null
      localStorage.removeItem('token')
    },
  },
})
