const SESSION_KEY = 'norix_session_token'
const USER_KEY = 'norix_session_user'

export interface SessionUser {
  id: string
  email: string
  name: string
  role: number
}

export const sessionStore = {
  save(token: string, user: SessionUser) {
    localStorage.setItem(SESSION_KEY, token)
    localStorage.setItem(USER_KEY, JSON.stringify(user))
  },
  getToken() {
    return localStorage.getItem(SESSION_KEY)
  },
  getUser(): SessionUser | null {
    const value = localStorage.getItem(USER_KEY)
    if (!value) return null
    try {
      return JSON.parse(value) as SessionUser
    } catch {
      this.clear()
      return null
    }
  },
  clear() {
    localStorage.removeItem(SESSION_KEY)
    localStorage.removeItem(USER_KEY)
  },
}
