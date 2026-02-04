const env = import.meta.env
const useProxy = env.VITE_API_PROXY === '1'
const defaultProtocol = env.VITE_TLS === '1' ? 'https' : 'http'
const defaultBase = `${defaultProtocol}://localhost:8080`
const apiTarget = env.VITE_API_BASE_URL || defaultBase

export const API_BASE_URL = useProxy ? '' : apiTarget
export const API_TARGET = apiTarget

function buildUrl(path: string) {
  return `${API_BASE_URL}${path}`
}

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const resp = await fetch(buildUrl(path), {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`request failed ${resp.status}: ${text}`)
  }
  return resp.json()
}

async function requestText(path: string, init: RequestInit = {}): Promise<string> {
  const resp = await fetch(buildUrl(path), init)
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`request failed ${resp.status}: ${text}`)
  }
  return resp.text()
}

async function requestNoContent(path: string, init: RequestInit = {}): Promise<void> {
  const resp = await fetch(buildUrl(path), init)
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`request failed ${resp.status}: ${text}`)
  }
}

export async function getDevices() {
  return requestJson('/api/v1/devices')
}

export async function getDevice(deviceId: string) {
  return requestJson(`/api/v1/devices/${encodeURIComponent(deviceId)}`)
}

export async function deleteDevice(deviceId: string) {
  return requestNoContent(`/api/v1/devices/${encodeURIComponent(deviceId)}`, {
    method: 'DELETE',
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

export async function setDesiredStateGroup(groupId: string, payload: Record<string, unknown>) {
  return requestJson(`/api/v1/desired-state/groups/${encodeURIComponent(groupId)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function uploadArtifact(formData: FormData) {
  const resp = await fetch(buildUrl('/api/v1/artifacts/upload'), {
    method: 'POST',
    body: formData,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`upload failed ${resp.status}: ${text}`)
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

export async function getDeviceLogs(deviceId: string) {
  return requestText(`/api/v1/logs/${encodeURIComponent(deviceId)}`)
}
