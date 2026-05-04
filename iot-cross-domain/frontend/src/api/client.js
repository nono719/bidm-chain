const DEFAULT_BASE_URL = 'http://localhost:8080'
const USER_KEY = 'bidm.user'
const TOKEN_KEY = 'bidm.token'

function getSessionStore() {
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

export function getBaseURL() {
  const v = localStorage.getItem('bidm.baseURL')
  return v || DEFAULT_BASE_URL
}

export function setBaseURL(v) {
  localStorage.setItem('bidm.baseURL', v)
}

export function getToken() {
  const store = getSessionStore()
  if (!store) return ''
  return store.getItem(TOKEN_KEY) || ''
}

export function setToken(v) {
  const store = getSessionStore()
  if (!store) return
  if (!v) {
    store.removeItem(TOKEN_KEY)
    return
  }
  store.setItem(TOKEN_KEY, v)
}

export function getUser() {
  const store = getSessionStore()
  if (!store) return null
  const raw = store.getItem(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

export function setUser(u) {
  const store = getSessionStore()
  if (!store) return
  if (!u) {
    store.removeItem(USER_KEY)
    return
  }
  store.setItem(USER_KEY, JSON.stringify(u))
}

export async function apiRequest(path, { method = 'GET', body, headers } = {}) {
  const h = { 'Content-Type': 'application/json', ...(headers || {}) }
  const token = getToken()
  if (token) h.Authorization = `Bearer ${token}`

  const res = await fetch(`${getBaseURL()}${path}`, {
    method,
    headers: h,
    body: body ? JSON.stringify(body) : undefined
  })

  const json = await res.json().catch(() => null)
  if (!json) {
    return { code: 500, message: 'invalid response' }
  }
  return json
}
