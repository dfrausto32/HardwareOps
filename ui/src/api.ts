const env = import.meta.env
const simulateProd = env.VITE_SIMULATE_PROD === '1'
const useProxy = env.VITE_API_PROXY === '1' && !simulateProd
const defaultProtocol = env.VITE_TLS === '1' ? 'https' : 'http'
const runtimeOrigin =
  typeof window !== 'undefined' && window.location?.origin ? window.location.origin : ''
const defaultBase = runtimeOrigin || `${defaultProtocol}://localhost:8080`
const apiTarget = env.VITE_API_BASE_URL || defaultBase
let authToken = env.VITE_AUTH_TOKEN || ''

export const API_BASE_URL = useProxy ? '' : apiTarget
export const API_TARGET = apiTarget
const AUTH_STORAGE_KEY = 'hwops_auth_token'
const AUTH_EXPIRED_EVENT = 'hwops:auth-expired'

export function getAuthToken() {
  if (authToken) return authToken
  if (typeof window !== 'undefined') {
    const stored = window.localStorage.getItem(AUTH_STORAGE_KEY) || ''
    authToken = stored
  }
  return authToken
}

export function setAuthToken(token: string) {
  authToken = token || ''
  if (typeof window !== 'undefined') {
    if (authToken) {
      window.localStorage.setItem(AUTH_STORAGE_KEY, authToken)
    } else {
      window.localStorage.removeItem(AUTH_STORAGE_KEY)
    }
  }
}

function emitAuthExpired(detail: { path: string; status: number }) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(AUTH_EXPIRED_EVENT, { detail }))
}

function handleUnauthorized(path: string, status: number) {
  if (status !== 401) return
  if (path.startsWith('/api/v1/auth/login') || path.startsWith('/api/v1/auth/register')) return
  if (!getAuthToken()) return
  setAuthToken('')
  emitAuthExpired({ path, status })
}

export function subscribeAuthExpired(listener: (detail: { path: string; status: number }) => void) {
  if (typeof window === 'undefined') {
    return () => {}
  }
  const handler = (event: Event) => {
    const custom = event as CustomEvent<{ path: string; status: number }>
    listener(custom.detail || { path: '', status: 401 })
  }
  window.addEventListener(AUTH_EXPIRED_EVENT, handler as EventListener)
  return () => window.removeEventListener(AUTH_EXPIRED_EVENT, handler as EventListener)
}

function buildUrl(path: string) {
  return `${API_BASE_URL}${path}`
}

function buildHeaders(initHeaders?: HeadersInit, extra?: Record<string, string>) {
  const headers = new Headers(initHeaders || {})
  if (extra) {
    Object.entries(extra).forEach(([key, value]) => headers.set(key, value))
  }
  const token = getAuthToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  return headers
}

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const resp = await fetch(buildUrl(path), {
    ...init,
    headers: buildHeaders(init.headers, { 'Content-Type': 'application/json' }),
  })
  if (!resp.ok) {
    const text = await resp.text()
    handleUnauthorized(path, resp.status)
    const err = new Error(`request failed ${resp.status}: ${text}`) as Error & { status?: number }
    err.status = resp.status
    throw err
  }
  return resp.json()
}

async function requestText(path: string, init: RequestInit = {}): Promise<string> {
  const resp = await fetch(buildUrl(path), {
    ...init,
    headers: buildHeaders(init.headers),
  })
  if (!resp.ok) {
    const text = await resp.text()
    handleUnauthorized(path, resp.status)
    const err = new Error(`request failed ${resp.status}: ${text}`) as Error & { status?: number }
    err.status = resp.status
    throw err
  }
  return resp.text()
}

async function requestNoContent(path: string, init: RequestInit = {}): Promise<void> {
  const resp = await fetch(buildUrl(path), {
    ...init,
    headers: buildHeaders(init.headers),
  })
  if (!resp.ok) {
    const text = await resp.text()
    handleUnauthorized(path, resp.status)
    const err = new Error(`request failed ${resp.status}: ${text}`) as Error & { status?: number }
    err.status = resp.status
    throw err
  }
}

export async function getDevices() {
  return requestJson('/api/v1/devices')
}

export async function getDevice(deviceId: string) {
  return requestJson(`/api/v1/devices/${encodeURIComponent(deviceId)}`)
}

export async function decommissionDevice(deviceId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/devices/${encodeURIComponent(deviceId)}/decommission`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function patchDevice(deviceId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/devices/${encodeURIComponent(deviceId)}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export async function listGroups() {
  return requestJson('/api/v1/groups')
}

export async function putGroup(groupId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/groups/${encodeURIComponent(groupId)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteGroup(groupId: string) {
  return requestNoContent(`/api/v1/groups/${encodeURIComponent(groupId)}`, {
    method: 'DELETE',
  })
}

export async function batchGroups(payload: Record<string, unknown>) {
  return requestJson('/api/v1/groups/batch', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function getDesiredState() {
  return requestJson('/api/v1/desired-state')
}

export async function setDesiredStateDevice(deviceId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/desired-state/devices/${encodeURIComponent(deviceId)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function clearDesiredStateDevice(deviceId: string) {
  return requestNoContent(`/api/v1/desired-state/devices/${encodeURIComponent(deviceId)}`, {
    method: 'DELETE',
  })
}

export async function setDesiredStateGroup(groupId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/desired-state/groups/${encodeURIComponent(groupId)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function clearDesiredStateGroup(groupId: string) {
  return requestNoContent(`/api/v1/desired-state/groups/${encodeURIComponent(groupId)}`, {
    method: 'DELETE',
  })
}

export async function uploadArtifact(formData: FormData) {
  const resp = await fetch(buildUrl('/api/v1/artifacts/upload'), {
    method: 'POST',
    body: formData,
    headers: buildHeaders(),
  })
  if (!resp.ok) {
    const text = await resp.text()
    handleUnauthorized('/api/v1/artifacts/upload', resp.status)
    const err = new Error(`upload failed ${resp.status}: ${text}`) as Error & { status?: number }
    err.status = resp.status
    throw err
  }
  return resp.json()
}

export async function listArtifacts() {
  return requestJson('/api/v1/artifacts')
}

export async function deleteArtifact(artifactId: string) {
  return requestNoContent(`/api/v1/artifacts/${encodeURIComponent(artifactId)}`, {
    method: 'DELETE',
  })
}

export async function deprecateArtifact(artifactId: string, deleteAfterDays?: number) {
  const payload: Record<string, unknown> = {}
  if (deleteAfterDays && deleteAfterDays > 0) {
    payload.deleteAfterDays = deleteAfterDays
  }
  return requestJson(`/api/v1/artifacts/${encodeURIComponent(artifactId)}/deprecate`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function restoreArtifact(artifactId: string) {
  return requestJson(`/api/v1/artifacts/${encodeURIComponent(artifactId)}/restore`, {
    method: 'POST',
  })
}

export async function getArtifactLifecyclePolicy() {
  return requestJson('/api/v1/artifacts/lifecycle/policy')
}

export async function getArtifactLifecycleStatus() {
  return requestJson('/api/v1/artifacts/lifecycle/status')
}

export async function setArtifactLifecyclePolicy(deprecatedDeleteAfterDays: number) {
  return requestJson('/api/v1/artifacts/lifecycle/policy', {
    method: 'PUT',
    body: JSON.stringify({ deprecatedDeleteAfterDays }),
  })
}

export async function pruneArtifacts(limit = 100) {
  const qs = new URLSearchParams()
  if (limit > 0) qs.set('limit', String(limit))
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  return requestJson(`/api/v1/artifacts/lifecycle/prune${suffix}`, {
    method: 'POST',
  })
}

export async function getReleaseAutoUpdateStatus() {
  return requestJson('/api/v1/release-auto-update')
}

export async function setReleaseAutoUpdateSettings(payload: { enabled: boolean; allowUnsigned: boolean }) {
  return requestJson('/api/v1/release-auto-update', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function runReleaseAutoUpdate() {
  return requestJson('/api/v1/release-auto-update/run', {
    method: 'POST',
  })
}

export async function getDeviceLogs(deviceId: string) {
  return requestText(`/api/v1/logs/${encodeURIComponent(deviceId)}`)
}

export async function listAuditEvents(params: Record<string, string | number | undefined> = {}) {
  const qs = new URLSearchParams()
  Object.entries(params).forEach(([key, val]) => {
    if (val === undefined || val === null || val === '') return
    qs.set(key, String(val))
  })
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  return requestJson(`/api/v1/audit${suffix}`)
}

export async function listRuntimeEvents(params: Record<string, string | number | undefined> = {}) {
  const qs = new URLSearchParams()
  Object.entries(params).forEach(([key, val]) => {
    if (val === undefined || val === null || val === '') return
    qs.set(key, String(val))
  })
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  return requestJson(`/api/v1/events/history${suffix}`)
}

export async function getEventRetention() {
  return requestJson('/api/v1/events/retention')
}

export async function setEventRetention(days: number) {
  return requestJson('/api/v1/events/retention', {
    method: 'PUT',
    body: JSON.stringify({ days }),
  })
}

export async function listPendingEnrollments(params: Record<string, string | number | undefined> = {}) {
  const qs = new URLSearchParams()
  Object.entries(params).forEach(([key, val]) => {
    if (val === undefined || val === null || val === '') return
    qs.set(key, String(val))
  })
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  return requestJson(`/api/v1/pending-enrollments${suffix}`)
}

export async function listEnrollmentProfiles() {
  return requestJson('/api/v1/enrollment-profiles')
}

export async function createEnrollmentProfile(payload: Record<string, unknown>) {
  return requestJson('/api/v1/enrollment-profiles', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updateEnrollmentProfile(profileId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/enrollment-profiles/${encodeURIComponent(profileId)}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export async function rotateEnrollmentProfile(profileId: string) {
  return requestJson(`/api/v1/enrollment-profiles/${encodeURIComponent(profileId)}/rotate`, {
    method: 'POST',
  })
}

export async function disableEnrollmentProfile(profileId: string) {
  return requestJson(`/api/v1/enrollment-profiles/${encodeURIComponent(profileId)}/disable`, {
    method: 'POST',
  })
}

export async function enableEnrollmentProfile(profileId: string) {
  return requestJson(`/api/v1/enrollment-profiles/${encodeURIComponent(profileId)}/enable`, {
    method: 'POST',
  })
}

export async function approvePendingEnrollment(requestId: string) {
  return requestJson(`/api/v1/pending-enrollments/${encodeURIComponent(requestId)}/approve`, {
    method: 'POST',
  })
}

export async function denyPendingEnrollment(requestId: string, reason = '') {
  return requestJson(`/api/v1/pending-enrollments/${encodeURIComponent(requestId)}/deny`, {
    method: 'POST',
    body: JSON.stringify({ reason }),
  })
}

export async function resetPendingEnrollment(requestId: string) {
  return requestJson(`/api/v1/pending-enrollments/${encodeURIComponent(requestId)}/reset`, {
    method: 'POST',
  })
}

export async function downloadAuditCSV(params: Record<string, string | number | undefined> = {}) {
  const qs = new URLSearchParams()
  Object.entries(params).forEach(([key, val]) => {
    if (val === undefined || val === null || val === '') return
    qs.set(key, String(val))
  })
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  return requestText(`/api/v1/audit.csv${suffix}`)
}

export async function getAuditRetention() {
  return requestJson('/api/v1/audit/retention')
}

export async function setAuditRetention(days: number) {
  return requestJson('/api/v1/audit/retention', {
    method: 'PUT',
    body: JSON.stringify({ days }),
  })
}

export async function getMaintenance() {
  return requestJson('/api/v1/maintenance')
}

export async function setMaintenance(payload: Record<string, unknown>) {
  return requestJson('/api/v1/maintenance', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function getUpgradeStatus() {
  return requestJson('/api/v1/maintenance/upgrade')
}

export async function getUpgradeAvailable() {
  return requestJson('/api/v1/maintenance/upgrade/available')
}

export async function getUpgradePreflight() {
  return requestJson('/api/v1/maintenance/upgrade/preflight')
}

export async function applyUpgrade() {
  return requestJson('/api/v1/maintenance/upgrade', {
    method: 'POST',
  })
}

export async function getRotationStatus() {
  return requestJson('/api/v1/cert-rotation')
}

export async function reloadRotation() {
  return requestJson('/api/v1/cert-rotation/reload', {
    method: 'POST',
  })
}

export async function rotateRotation() {
  return requestJson('/api/v1/cert-rotation/rotate', {
    method: 'POST',
  })
}

export async function cleanupRotation() {
  return requestJson('/api/v1/cert-rotation/cleanup', {
    method: 'POST',
  })
}

export async function listBackups() {
  return requestJson('/api/v1/maintenance/backups')
}

export async function getBackupStatus() {
  return requestJson('/api/v1/maintenance/backup')
}

export async function startBackup() {
  return requestJson('/api/v1/maintenance/backup', {
    method: 'POST',
  })
}

export async function getRestoreStatus() {
  return requestJson('/api/v1/maintenance/restore')
}

export async function startRestore(id: string) {
  return requestJson('/api/v1/maintenance/restore', {
    method: 'POST',
    body: JSON.stringify({ id, wipe: true }),
  })
}

export async function getHealthSummary() {
  return requestJson('/api/v1/health/summary')
}

export async function getMetricsText() {
  return requestText('/metrics')
}

export async function login(email: string, password: string) {
  return requestJson('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export async function getMe() {
  return requestJson('/api/v1/auth/me')
}

export async function getAuthStatus() {
  return requestJson('/api/v1/auth/status')
}

export async function getBootstrapStatus() {
  return requestJson('/api/v1/bootstrap')
}

export async function downloadBootstrapCA(token?: string) {
  const headers: Record<string, string> = {}
  if (token) {
    headers['X-Bootstrap-Token'] = token
  }
  const resp = await fetch(buildUrl('/api/v1/bootstrap/ca'), {
    method: 'GET',
    headers: buildHeaders(headers),
  })
  if (!resp.ok) {
    const text = await resp.text()
    handleUnauthorized('/api/v1/bootstrap/ca', resp.status)
    const err = new Error(`request failed ${resp.status}: ${text}`) as Error & { status?: number }
    err.status = resp.status
    throw err
  }
  return resp.blob()
}

export async function registerWithVoucher(payload: Record<string, unknown>) {
  return requestJson('/api/v1/auth/register', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function createVoucher(payload: Record<string, unknown>) {
  return requestJson('/api/v1/auth/vouchers', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function listUsers() {
  return requestJson('/api/v1/users')
}

export async function createUser(payload: Record<string, unknown>) {
  return requestJson('/api/v1/users', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}
