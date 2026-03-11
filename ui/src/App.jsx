import { useEffect, useMemo, useRef, useState } from 'react'
import {
  getDevices,
  getDevice,
  listGroups,
  putGroup,
  deleteGroup,
  batchGroups,
  patchDevice,
  clearDesiredStateDevice,
  getDesiredState,
  listArtifacts,
  uploadArtifact,
  deprecateArtifact,
  restoreArtifact,
  getArtifactLifecyclePolicy,
  getArtifactLifecycleStatus,
  setArtifactLifecyclePolicy,
  pruneArtifacts,
  getReleaseAutoUpdateStatus,
  setReleaseAutoUpdateSettings,
  runReleaseAutoUpdate,
  setDesiredStateDevice,
  setDesiredStateGroup,
  clearDesiredStateGroup,
  getDeviceLogs,
  decommissionDevice,
  deleteArtifact,
  getMaintenance,
  setMaintenance,
  getUpgradeStatus,
  getUpgradeAvailable,
  getUpgradePreflight,
  applyUpgrade,
  listBackups,
  getBackupStatus,
  startBackup,
  getRestoreStatus,
  startRestore,
  getHealthSummary,
  getRotationStatus,
  reloadRotation,
  rotateRotation,
  cleanupRotation,
  listAuditEvents,
  downloadAuditCSV,
  getAuditRetention,
  setAuditRetention,
  listRuntimeEvents,
  getEventRetention,
  setEventRetention,
  getMetricsText,
  login as apiLogin,
  getMe,
  getAuthStatus,
  getBootstrapStatus,
  downloadBootstrapCA,
  registerWithVoucher,
  createVoucher,
  listUsers,
  createUser,
  listEnrollmentProfiles,
  createEnrollmentProfile,
  updateEnrollmentProfile,
  rotateEnrollmentProfile,
  disableEnrollmentProfile,
  enableEnrollmentProfile,
  listPendingEnrollments,
  approvePendingEnrollment,
  denyPendingEnrollment,
  resetPendingEnrollment,
  getAuthToken,
  setAuthToken as persistAuthToken,
  subscribeAuthExpired,
} from './api'
import { buildPermissionState, firstAllowedKey } from './rbac'

const nav = [
  { id: 'dashboard', label: 'Dashboard', icon: 'icon-dashboard' },
  { id: 'metrics', label: 'Metrics', icon: 'icon-metrics' },
  { id: 'logs', label: 'Logs', icon: 'icon-logs' },
  { id: 'security', label: 'Security', icon: 'icon-security' },
  { id: 'settings', label: 'Settings', icon: 'icon-settings' },
]

const logsNav = [
  { id: 'events', label: 'Live Events' },
  { id: 'device', label: 'Device Logs' },
  { id: 'audit', label: 'Audit Log' },
]

const componentTypes = [
  { id: 'app_bundle', label: 'App Bundle' },
  { id: 'config_bundle', label: 'Config Bundle' },
  { id: 'data_bundle', label: 'Data Bundle' },
  { id: 'firmware', label: 'Firmware' },
  { id: 'container_image', label: 'Container Image' },
  { id: 'agent_bundle', label: 'Agent Bundle' },
]

const rangeOptions = [
  { id: '15m', label: '15m', ms: 15 * 60 * 1000 },
  { id: '1h', label: '1h', ms: 60 * 60 * 1000 },
  { id: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
  { id: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
]

const rangeMsById = rangeOptions.reduce((acc, item) => {
  acc[item.id] = item.ms
  return acc
}, {})

const chartColors = {
  total: '#8aa5ff',
  active: '#3bd487',
  degraded: '#f1c76f',
  stale: '#9aa3ad',
  offline: '#f27272',
  success: '#3bd487',
  error: '#f27272',
  warning: '#f1c76f',
  info: '#8aa5ff',
}

const autoTrackModes = [
  { id: 'inherit', label: 'Inherit' },
  { id: 'enabled', label: 'Enabled' },
  { id: 'disabled', label: 'Disabled' },
]

function parsePolicyObject(raw) {
  if (!raw) return {}
  if (typeof raw === 'object') return { ...raw }
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed
      }
    } catch {
      return {}
    }
  }
  return {}
}

function getAutoTrackModeFromPolicy(raw) {
  const policy = parsePolicyObject(raw)
  const mode = policy?.hwops?.autoVersion?.mode
  if (mode === 'enabled' || mode === 'disabled') return mode
  return 'inherit'
}

function mergeAutoTrackModeIntoPolicy(raw, mode) {
  const policy = parsePolicyObject(raw)
  const hwops = policy.hwops && typeof policy.hwops === 'object' ? { ...policy.hwops } : {}
  const autoVersion = hwops.autoVersion && typeof hwops.autoVersion === 'object' ? { ...hwops.autoVersion } : {}
  if (mode === 'enabled' || mode === 'disabled') {
    autoVersion.mode = mode
    hwops.autoVersion = autoVersion
    policy.hwops = hwops
    return policy
  }
  if (hwops.autoVersion) {
    delete hwops.autoVersion
  }
  if (Object.keys(hwops).length > 0) {
    policy.hwops = hwops
  } else {
    delete policy.hwops
  }
  return policy
}

function formatChartValue(value, integerOnly) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  if (integerOnly) return Math.round(value).toLocaleString()
  return Number(value).toLocaleString()
}

function TimeSeriesChart({ series = [], rangeMs = rangeMsById['1h'], height = 180, integerOnly = false }) {
  const now = Date.now()
  const startTs = now - rangeMs
  const width = 1000
  const padding = 10

  const normalizedSeries = series.map((entry) => ({
    ...entry,
    points: (entry.points || []).filter((point) => point.ts >= startTs),
  }))
  const allPoints = normalizedSeries.flatMap((entry) => entry.points || [])
  if (allPoints.length === 0) {
    return <div className="placeholder">No data yet.</div>
  }
  const values = allPoints.map((point) => Number(point.value || 0))
  let minY = Math.min(...values)
  let maxY = Math.max(...values)
  if (!Number.isFinite(minY) || !Number.isFinite(maxY)) {
    minY = 0
    maxY = 1
  }
  minY = Math.min(0, minY)
  if (integerOnly) {
    minY = Math.floor(minY)
    maxY = Math.ceil(maxY)
  }
  if (minY === maxY) {
    maxY = minY + 1
  }
  const tickCount = 4
  let tickStep = (maxY - minY) / tickCount
  if (integerOnly) {
    tickStep = Math.max(1, Math.ceil(tickStep))
    maxY = minY + tickStep * tickCount
  }
  const ticks = Array.from({ length: tickCount + 1 }, (_, idx) => maxY - idx * tickStep)

  const xFor = (ts) => {
    const ratio = Math.min(Math.max((ts - startTs) / rangeMs, 0), 1)
    return padding + ratio * (width - padding * 2)
  }
  const yFor = (value) => {
    const ratio = (Number(value || 0) - minY) / (maxY - minY)
    return height - padding - ratio * (height - padding * 2)
  }

  return (
    <div className="chart">
      <div className="chart-body">
        <div className="chart-y" style={{ gridTemplateRows: `repeat(${ticks.length}, 1fr)` }}>
          {ticks.map((tick) => (
            <span key={tick}>{formatChartValue(tick, integerOnly)}</span>
          ))}
        </div>
        <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
          <rect className="chart-bg" x="0" y="0" width={width} height={height} />
          {ticks.map((tick) => {
            const y = yFor(tick)
            return (
              <line
                key={`grid-${tick}`}
                className="chart-grid"
                x1={padding}
                x2={width - padding}
                y1={y}
                y2={y}
              />
            )
          })}
          {normalizedSeries.map((entry) => (
            <g key={entry.id}>
              <polyline
                className="chart-line"
                stroke={entry.color || '#999'}
                points={(entry.points || [])
                  .map((point) => `${xFor(point.ts)},${yFor(point.value)}`)
                  .join(' ')}
              />
              {entry.points && entry.points.length > 0 && (() => {
                const lastPoint = entry.points[entry.points.length - 1]
                const x = xFor(lastPoint.ts)
                const y = yFor(lastPoint.value)
                const labelText = formatChartValue(lastPoint.value, integerOnly)
                const anchor = x > width - 80 ? 'end' : 'start'
                const xLabel = anchor === 'end' ? x - 6 : x + 6
                return (
                  <text
                    className="chart-last-label"
                    x={xLabel}
                    y={y}
                    textAnchor={anchor}
                    alignmentBaseline="middle"
                    fill={entry.color || '#fff'}
                  >
                    {labelText}
                  </text>
                )
              })()}
            </g>
          ))}
        </svg>
      </div>
      <div className="chart-footer">
        <span>{new Date(startTs).toLocaleTimeString()}</span>
        <span>Now</span>
      </div>
    </div>
  )
}

function ChartLegend({ series }) {
  if (!series || series.length === 0) return null
  return (
    <div className="chart-legend">
      {series.map((entry) => (
        <div key={entry.id} className="legend-item">
          <span className="legend-dot" style={{ background: entry.color }} />
          {entry.label}
        </div>
      ))}
    </div>
  )
}

function newComponentRow(overrides = {}) {
  return {
    id: typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : `cmp-${Date.now()}-${Math.random()}`,
    key: '',
    artifactType: '',
    artifactId: '',
    desiredVersion: '',
    desiredConfigRev: '',
    policy: {},
    autoTrackMode: 'inherit',
    locked: false,
    ...overrides,
  }
}

function normalizeArtifactType(val) {
  if (!val) return ''
  return String(val).trim().toLowerCase()
}

function normalizeArtifactStatus(val) {
  const normalized = String(val || '').trim().toLowerCase()
  if (!normalized) return 'active'
  return normalized
}

function filterArtifactGroupsByType(groups, type) {
  const target = normalizeArtifactType(type)
  if (!target) return groups
  return groups
    .map((group) => ({
      ...group,
      versions: group.versions.filter((artifact) => normalizeArtifactType(artifact.type) === target),
    }))
    .filter((group) => group.versions.length > 0)
}

function fallbackComponentType(key, artifactType) {
  const normalized = normalizeArtifactType(artifactType)
  if (normalized) return normalized
  if (key === 'agent_bundle') return 'agent_bundle'
  if (key === 'app_bundle') return 'app_bundle'
  return ''
}

function parseCSVLine(line) {
  const out = []
  let cur = ''
  let inQuotes = false

  for (let i = 0; i < line.length; i += 1) {
    const ch = line[i]
    if (inQuotes) {
      if (ch === '"') {
        if (line[i + 1] === '"') {
          cur += '"'
          i += 1
        } else {
          inQuotes = false
        }
      } else {
        cur += ch
      }
    } else if (ch === '"') {
      inQuotes = true
    } else if (ch === ',') {
      out.push(cur)
      cur = ''
    } else {
      cur += ch
    }
  }
  out.push(cur)
  return out
}

function parseCSV(text) {
  const trimmed = text.trim()
  if (!trimmed) return { header: [], rows: [] }
  const lines = trimmed.split(/\r?\n/)
  const header = parseCSVLine(lines[0])
  const rows = lines.slice(1).map(parseCSVLine)
  return { header, rows }
}

function normalizeObject(value) {
  if (!value) return {}
  if (typeof value === 'object') return value
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed
      }
    } catch {
      return {}
    }
  }
  return {}
}

function selectorMatches(labels, selector) {
  if (!selector || Object.keys(selector).length === 0) return true
  if (!labels) return false
  return Object.entries(selector).every(([key, val]) => labels[key] === val)
}

function selectorToForm(selector) {
  const region = selector.region ?? ''
  const role = selector.role ?? ''
  const site = selector.site ?? ''
  const custom = Object.entries(selector)
    .filter(([key]) => !['region', 'role', 'site'].includes(key))
    .map(([key, value]) => ({ key, value: String(value ?? '') }))
  return { region, role, site, custom }
}

function objectToKeyValueRows(obj) {
  const normalized = normalizeObject(obj)
  const entries = Object.entries(normalized)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => ({ key: String(key), value: String(value ?? '') }))
  return entries.length > 0 ? entries : [{ key: '', value: '' }]
}

function keyValueRowsToObject(rows) {
  const out = {}
  const seen = new Set()
  for (const row of rows || []) {
    const key = String(row?.key || '').trim()
    if (!key) continue
    if (seen.has(key)) {
      return { value: null, error: `Duplicate label key: ${key}` }
    }
    seen.add(key)
    out[key] = String(row?.value ?? '').trim()
  }
  return { value: out, error: '' }
}

function formatSelector(selector) {
  const entries = Object.entries(selector || {})
  if (entries.length === 0) return '—'
  return entries.map(([key, value]) => `${key}=${value}`).join(', ')
}

function normalizeSelector(selector) {
  const normalized = {}
  const obj = normalizeObject(selector)
  Object.entries(obj).forEach(([key, value]) => {
    const k = String(key || '').trim()
    if (!k) return
    normalized[k] = String(value ?? '')
  })
  return normalized
}

function stableSelectorString(selector) {
  const sorted = Object.entries(selector || {}).sort(([a], [b]) => a.localeCompare(b))
  return JSON.stringify(Object.fromEntries(sorted))
}

function isUUID(value) {
  const input = String(value || '').trim()
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(input)
}

function csvEscape(value) {
  const input = String(value ?? '')
  if (!/[",\n]/.test(input)) return input
  return `"${input.replace(/"/g, '""')}"`
}

function buildBulkGroupsRollbackCsv(rows) {
  const header = ['action', 'groupId', 'name', 'selector_json']
  const lines = [header.join(',')]
  rows.forEach((row) => {
    const selectorJson = row.action === 'upsert' ? JSON.stringify(row.selector || {}) : ''
    lines.push([
      csvEscape(row.action || ''),
      csvEscape(row.groupId || ''),
      csvEscape(row.name || ''),
      csvEscape(selectorJson),
    ].join(','))
  })
  return `${lines.join('\n')}\n`
}

function parseBulkGroupsCsv(csvText, existingGroups) {
  const parsed = parseCSV(csvText || '')
  const headers = (parsed.header || []).map((h) => String(h || '').trim())
  const headersLower = headers.map((h) => h.toLowerCase())
  const getIdx = (candidates) => {
    for (const candidate of candidates) {
      const idx = headersLower.indexOf(candidate)
      if (idx >= 0) return idx
    }
    return -1
  }
  const actionIdx = getIdx(['action'])
  const groupIDIdx = getIdx(['groupid', 'group_id', 'id'])
  const nameIdx = getIdx(['name'])
  const regionIdx = getIdx(['region'])
  const roleIdx = getIdx(['role'])
  const siteIdx = getIdx(['site'])
  const selectorIdx = getIdx(['selector', 'selector_json'])
  const dynamicSelectorColumns = headers
    .map((header, idx) => ({ header, idx, lower: headersLower[idx] }))
    .filter((entry) => entry.lower.startsWith('selector.'))
  const existingByID = new Map(
    (existingGroups || []).map((group) => [
      group.groupId,
      { name: group.name || '', selector: normalizeSelector(group.selector) },
    ]),
  )

  const errors = []
  const rows = []
  let rowCounter = 0

  ;(parsed.rows || []).forEach((lineRow, lineIdx) => {
    const line = lineIdx + 2
    const cells = lineRow.map((cell) => String(cell || ''))
    const cellAt = (idx) => (idx >= 0 && idx < cells.length ? cells[idx].trim() : '')
    const hasAny = cells.some((cell) => String(cell || '').trim() !== '')
    if (!hasAny) return

    const action = (cellAt(actionIdx) || 'upsert').toLowerCase()
    const baseGroupID = cellAt(groupIDIdx)
    const name = cellAt(nameIdx)
    let groupId = baseGroupID
    const selector = {}
    let rowError = ''

    if (action !== 'upsert' && action !== 'delete') {
      rowError = `line ${line}: action must be upsert or delete`
    }

    if (action === 'delete') {
      if (!groupId) {
        rowError = `line ${line}: groupId is required for delete`
      }
    } else if (!groupId) {
      groupId = crypto.randomUUID()
    }

    if (!rowError && groupId && !isUUID(groupId)) {
      rowError = `line ${line}: groupId must be a UUID`
    }

    if (!rowError && action === 'upsert') {
      const selectorJson = cellAt(selectorIdx)
      if (selectorJson) {
        try {
          const parsedSelector = JSON.parse(selectorJson)
          if (!parsedSelector || typeof parsedSelector !== 'object' || Array.isArray(parsedSelector)) {
            rowError = `line ${line}: selector_json must be a JSON object`
          } else {
            Object.entries(parsedSelector).forEach(([key, value]) => {
              selector[String(key)] = String(value ?? '')
            })
          }
        } catch {
          rowError = `line ${line}: selector_json is invalid JSON`
        }
      }
      if (!rowError) {
        const region = cellAt(regionIdx)
        const role = cellAt(roleIdx)
        const site = cellAt(siteIdx)
        if (region) selector.region = region
        if (role) selector.role = role
        if (site) selector.site = site
        dynamicSelectorColumns.forEach(({ header, idx }) => {
          const value = cellAt(idx)
          if (!value) return
          const key = header.slice(header.indexOf('.') + 1).trim()
          if (!key) return
          selector[key] = value
        })
      }
    }

    const existing = existingByID.get(groupId)
    let outcome = 'error'
    if (!rowError) {
      if (action === 'delete') {
        outcome = existing ? 'delete' : 'skip-not-found'
      } else if (!existing) {
        outcome = 'create'
      } else {
        const beforeName = existing.name || ''
        const beforeSelector = stableSelectorString(existing.selector || {})
        const afterSelector = stableSelectorString(selector)
        outcome = beforeName === name && beforeSelector === afterSelector ? 'no-change' : 'update'
      }
    } else {
      errors.push(rowError)
    }

    rowCounter += 1
    rows.push({
      rowId: `bulk-group-row-${rowCounter}`,
      line,
      action,
      groupId,
      name,
      selector,
      outcome,
      error: rowError,
      applyStatus: '',
      applyError: '',
    })
  })

  const summary = {
    totalRows: rows.length,
    errorRows: rows.filter((row) => row.error).length,
    createRows: rows.filter((row) => row.outcome === 'create').length,
    updateRows: rows.filter((row) => row.outcome === 'update').length,
    deleteRows: rows.filter((row) => row.outcome === 'delete').length,
    noChangeRows: rows.filter((row) => row.outcome === 'no-change').length,
    skipNotFoundRows: rows.filter((row) => row.outcome === 'skip-not-found').length,
  }
  summary.applyRows = summary.createRows + summary.updateRows + summary.deleteRows
  summary.validRows = rows.length - summary.errorRows

  return { rows, errors, summary, headers }
}

function toIsoIfValid(value) {
  if (!value) return ''
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return ''
  return dt.toISOString()
}

function formatTime(value) {
  if (!value) return '—'
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return String(value)
  return dt.toLocaleString()
}

function formatNumber(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return Number(value).toLocaleString()
}

function formatBytes(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  const size = Number(value)
  if (size < 1024) return `${size} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let idx = -1
  let current = size
  while (current >= 1024 && idx < units.length - 1) {
    current /= 1024
    idx += 1
  }
  return `${current.toFixed(current >= 10 ? 0 : 1)} ${units[idx]}`
}

function formatDurationSeconds(seconds) {
  if (!seconds || seconds <= 0) return '—'
  let remaining = Math.floor(seconds)
  const days = Math.floor(remaining / 86400)
  remaining -= days * 86400
  const hours = Math.floor(remaining / 3600)
  remaining -= hours * 3600
  const minutes = Math.floor(remaining / 60)
  remaining -= minutes * 60
  const parts = []
  if (days) parts.push(`${days}d`)
  if (hours) parts.push(`${hours}h`)
  if (!days && minutes) parts.push(`${minutes}m`)
  if (!days && !hours && !minutes && remaining) parts.push(`${remaining}s`)
  return parts.join(' ') || '—'
}

async function copyText(text) {
  if (!text) return false
  if (navigator?.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return true
  }
  const input = document.createElement('textarea')
  input.value = text
  input.setAttribute('readonly', '')
  input.style.position = 'absolute'
  input.style.left = '-9999px'
  document.body.appendChild(input)
  input.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(input)
  return ok
}

function downloadTextFile(filename, text) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

function formatObjectSummary(value) {
  const obj = normalizeObject(value)
  const entries = Object.entries(obj)
  if (entries.length === 0) return '—'
  return entries.map(([key, val]) => `${key}=${String(val)}`).join(', ')
}

function parsePrometheusMetrics(text) {
  const values = {}
  const labeled = {}
  if (!text) return { values, labeled }
  const lines = text.split(/\r?\n/)
  for (const line of lines) {
    if (!line || line.startsWith('#')) continue
    const match = line.match(/^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+([-+eE0-9.]+)$/)
    if (!match) continue
    const name = match[1]
    const labelSet = match[2]
    const value = Number(match[3])
    if (Number.isNaN(value)) continue
    if (!labelSet) {
      values[name] = value
      continue
    }
    const labels = {}
    const body = labelSet.slice(1, -1)
    const labelRe = /([a-zA-Z_][a-zA-Z0-9_]*)="([^"]*)"/g
    let labelMatch
    while ((labelMatch = labelRe.exec(body))) {
      labels[labelMatch[1]] = labelMatch[2]
    }
    if (!labeled[name]) labeled[name] = []
    labeled[name].push({ labels, value })
  }
  return { values, labeled }
}

function sumLabeled(metrics, name) {
  const items = metrics.labeled?.[name] || []
  return items.reduce((acc, item) => acc + Number(item.value || 0), 0)
}

function groupLabeledMetrics(metrics, name, labelKey, mapLabel) {
  const items = metrics.labeled?.[name] || []
  const out = {}
  items.forEach((item) => {
    const raw = item.labels?.[labelKey] || 'unknown'
    const key = mapLabel ? mapLabel(raw) : raw
    out[key] = (out[key] || 0) + Number(item.value || 0)
  })
  return out
}

export default function App() {
  const apiBaseUrl = useMemo(() => {
    if (import.meta.env.VITE_API_BASE_URL) return import.meta.env.VITE_API_BASE_URL
    if (typeof window !== 'undefined' && window.location?.origin) return window.location.origin
    return 'https://localhost:8080'
  }, [])
  const [authToken, setAuthToken] = useState(() => getAuthToken())
  const simulateProd = import.meta.env.VITE_SIMULATE_PROD === '1'
  const apiProxy = !simulateProd && import.meta.env.VITE_API_PROXY === '1'
  const wsBaseUrl = useMemo(() => {
    const override = import.meta.env.VITE_WS_BASE_URL
    if (override) return override
    if (apiProxy) {
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
      return `${proto}://${window.location.host}`
    }
    if (apiBaseUrl.startsWith('https://')) return apiBaseUrl.replace('https://', 'wss://')
    if (apiBaseUrl.startsWith('http://')) return apiBaseUrl.replace('http://', 'ws://')
    return apiBaseUrl
  }, [apiBaseUrl, apiProxy])
  const wsUrl = useMemo(() => {
    const url = new URL(`${wsBaseUrl}/api/v1/events`)
    if (authToken) {
      url.searchParams.set('token', authToken)
    }
    return url.toString()
  }, [wsBaseUrl, authToken])

  const [authUser, setAuthUser] = useState(null)
  const [authUserLoaded, setAuthUserLoaded] = useState(false)
  const [authError, setAuthError] = useState('')
  const [loginForm, setLoginForm] = useState({ email: '', password: '' })
  const [loginStatus, setLoginStatus] = useState('')
  const [bootstrapState, setBootstrapState] = useState({
    enabled: false,
    authEnabled: false,
    tokenRequired: false,
    tokenHeader: '',
    caDownloadUrl: '',
  })
  const [bootstrapToken, setBootstrapToken] = useState('')
  const [bootstrapStatusMessage, setBootstrapStatusMessage] = useState('')
  const [authView, setAuthView] = useState('login')
  const [authStatus, setAuthStatus] = useState({ enabled: false, mode: 'disabled', loaded: false })
  const permissions = useMemo(
    () => buildPermissionState({ authEnabled: authStatus.enabled, roles: authUser?.roles || [] }),
    [authStatus.enabled, authUser],
  )
  const [registerForm, setRegisterForm] = useState({
    token: '',
    email: '',
    password: '',
    displayName: '',
  })
  const [registerStatus, setRegisterStatus] = useState('')
  const [users, setUsers] = useState([])
  const [usersError, setUsersError] = useState('')
  const [usersStatus, setUsersStatus] = useState('')
  const [userForm, setUserForm] = useState({
    email: '',
    password: '',
    displayName: '',
    roles: ['viewer'],
  })
  const [voucherForm, setVoucherForm] = useState({
    email: '',
    ttlHours: '24',
    roles: ['viewer'],
  })
  const [voucherToken, setVoucherToken] = useState('')
  const [voucherStatus, setVoucherStatus] = useState('')

  const [view, setView] = useState(() => {
    const hash = window.location.hash.replace('#', '')
    return hash || 'dashboard'
  })
  const visibleNav = useMemo(
    () => nav.filter((item) => permissions.views[item.id]),
    [permissions],
  )
  const visibleLogsTabs = useMemo(
    () => logsNav.filter((item) => permissions.logsTabs[item.id]),
    [permissions],
  )
  const fallbackView = useMemo(
    () => firstAllowedKey(permissions.views, 'dashboard'),
    [permissions],
  )
  const fallbackLogsTab = useMemo(
    () => firstAllowedKey(permissions.logsTabs, 'events'),
    [permissions],
  )

  useEffect(() => {
    const onHash = () => {
      const hash = window.location.hash.replace('#', '')
      setView(hash || 'dashboard')
    }
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  useEffect(() => {
    // Handle OIDC callback: server redirects to /?oidc_token=<jwt>
    const params = new URLSearchParams(window.location.search)
    const oidcToken = params.get('oidc_token')
    if (oidcToken) {
      persistAuthToken(oidcToken)
      setAuthToken(oidcToken)
      // Remove oidc_token from the URL without triggering a reload.
      params.delete('oidc_token')
      const newSearch = params.toString()
      const newUrl = window.location.pathname + (newSearch ? '?' + newSearch : '') + window.location.hash
      window.history.replaceState({}, '', newUrl)
    }
  }, [])

  useEffect(() => {
    async function loadAuthStatus() {
      try {
        const res = await getAuthStatus()
        setAuthStatus({ ...res, loaded: true })
      } catch (err) {
        setAuthStatus({ enabled: false, mode: 'disabled', loaded: true })
      }
    }
    loadAuthStatus()
  }, [])

  useEffect(() => {
    async function loadBootstrapStatus() {
      try {
        const res = await getBootstrapStatus()
        setBootstrapState({
          enabled: Boolean(res?.enabled),
          authEnabled: Boolean(res?.authEnabled),
          tokenRequired: Boolean(res?.tokenRequired),
          tokenHeader: String(res?.tokenHeader || ''),
          caDownloadUrl: String(res?.caDownloadUrl || ''),
        })
      } catch {
        setBootstrapState({
          enabled: false,
          authEnabled: false,
          tokenRequired: false,
          tokenHeader: '',
          caDownloadUrl: '',
        })
      }
    }
    loadBootstrapStatus()
  }, [])

  useEffect(() => {
    async function loadMe() {
      if (!authToken) {
        setAuthUser(null)
        setAuthUserLoaded(true)
        return
      }
      setAuthUserLoaded(false)
      setAuthError('')
      try {
        const res = await getMe()
        setAuthUser(res)
        setAuthUserLoaded(true)
      } catch (err) {
        persistAuthToken('')
        setAuthToken('')
        setAuthUser(null)
        setAuthUserLoaded(true)
        setAuthError(err.message || String(err))
      }
    }
    loadMe()
  }, [authToken])

  useEffect(() => {
    if (!authToken) {
      setUsers([])
      return
    }
    loadUsers()
  }, [authToken, permissions.canManageUsers])

  useEffect(() => {
    if (!authStatus.loaded || permissions.views[view]) return
    setView(fallbackView)
    if (window.location.hash.replace('#', '') !== fallbackView) {
      window.location.hash = fallbackView
    }
  }, [authStatus.loaded, permissions, view, fallbackView])

  useEffect(() => {
    return subscribeAuthExpired(() => {
      persistAuthToken('')
      setAuthToken('')
      setAuthUser(null)
      setAuthView('login')
      setAuthError('Session expired. Please sign in again.')
      setLoginStatus('')
      setDevicesError('')
      setGroupsError('')
      setArtifactsError('')
      setDesiredError('')
      setLogError('')
      setAuditError('')
      setEventQueryError('')
      setEventsError('')
      setMaintenanceError('')
      setUpgradeError('')
      setUpgradeAvailableError('')
      setUpgradePreflightError('')
    })
  }, [])

  const [devices, setDevices] = useState([])
  const [devicesLoading, setDevicesLoading] = useState(false)
  const [devicesError, setDevicesError] = useState('')
  const [devicesStatus, setDevicesStatus] = useState('')
  const [deviceOrder, setDeviceOrder] = useState([])
  const [deviceStatusFilter, setDeviceStatusFilter] = useState('all')

  const [selectedDeviceId, setSelectedDeviceId] = useState('')
  const [selectedDeviceIds, setSelectedDeviceIds] = useState([])
  const [deviceDetail, setDeviceDetail] = useState(null)
  const [deviceDetailError, setDeviceDetailError] = useState('')
  const [deviceDrawerOpen, setDeviceDrawerOpen] = useState(false)
  const [drawerWidth, setDrawerWidth] = useState(() => {
    const stored = Number(localStorage.getItem('hwops-drawer-width') || '')
    return Number.isFinite(stored) && stored > 0 ? stored : 420
  })
  const [artifactModalOpen, setArtifactModalOpen] = useState(false)
  const [artifactPickerTarget, setArtifactPickerTarget] = useState(null)
  const [artifactUploadOpen, setArtifactUploadOpen] = useState(false)

  const [groups, setGroups] = useState([])
  const [groupsError, setGroupsError] = useState('')
  const [groupsStatus, setGroupsStatus] = useState('')
  const [groupForm, setGroupForm] = useState({
    groupId: '',
    name: '',
    region: '',
    role: '',
    site: '',
    custom: [],
  })
  const [groupModalOpen, setGroupModalOpen] = useState(false)
  const [selectedGroupId, setSelectedGroupId] = useState('')
  const [groupDesiredOpen, setGroupDesiredOpen] = useState(false)
  const [groupDesiredForm, setGroupDesiredForm] = useState({
    groupId: '',
    checkinIntervalSec: '',
    components: [newComponentRow()],
  })
  const [selectedGroupIds, setSelectedGroupIds] = useState([])
  const [groupBatchStatus, setGroupBatchStatus] = useState('')
  const [groupBatchError, setGroupBatchError] = useState('')
  const [groupMultiEditOpen, setGroupMultiEditOpen] = useState(false)
  const [groupMultiEditForm, setGroupMultiEditForm] = useState({
    region: '',
    role: '',
    site: '',
    custom: [{ key: '', value: '' }],
  })
  const [groupMultiEditStatus, setGroupMultiEditStatus] = useState('')
  const [groupMultiEditError, setGroupMultiEditError] = useState('')
  const [groupMultiDesiredOpen, setGroupMultiDesiredOpen] = useState(false)
  const [groupMultiDesiredForm, setGroupMultiDesiredForm] = useState({
    checkinIntervalSec: '',
    components: [newComponentRow()],
  })
  const [groupMultiDesiredStatus, setGroupMultiDesiredStatus] = useState('')
  const [groupMultiDesiredError, setGroupMultiDesiredError] = useState('')
  const [groupBulkOpen, setGroupBulkOpen] = useState(false)
  const [groupBulkCsv, setGroupBulkCsv] = useState('')
  const [groupBulkPreview, setGroupBulkPreview] = useState(null)
  const [groupBulkError, setGroupBulkError] = useState('')
  const [groupBulkStatus, setGroupBulkStatus] = useState('')
  const [groupBulkRollbackCsv, setGroupBulkRollbackCsv] = useState('')
  const [groupBulkApplying, setGroupBulkApplying] = useState(false)

  const [artifacts, setArtifacts] = useState([])
  const [selectedArtifactIds, setSelectedArtifactIds] = useState([])
  const [artifactsError, setArtifactsError] = useState('')
  const [artifactsStatus, setArtifactsStatus] = useState('')
  const [artifactLifecyclePolicy, setArtifactLifecyclePolicy] = useState({
    deprecatedDeleteAfterDays: 30,
    updatedAt: '',
  })
  const [artifactLifecyclePolicyInput, setArtifactLifecyclePolicyInput] = useState('30')
  const [artifactLifecycleStatus, setArtifactLifecycleStatus] = useState({
    enabled: false,
    running: false,
    intervalSeconds: 0,
    batchLimit: 200,
    alertReferenceThreshold: 10,
    lastRun: null,
    alerts: [],
  })
  const [releaseAutoUpdate, setReleaseAutoUpdate] = useState({
    enabled: false,
    allowUnsigned: false,
    intervalSeconds: 60,
    running: false,
    updatedAt: '',
    updatedByUserId: '',
    lastRun: null,
  })
  const [releaseAutoUpdateSaving, setReleaseAutoUpdateSaving] = useState(false)
  const [releaseAutoUpdateStatus, setReleaseAutoUpdateStatus] = useState('')

  const [desiredState, setDesiredState] = useState({ groups: [], devices: [] })
  const [desiredError, setDesiredError] = useState('')

  const [uploadStatus, setUploadStatus] = useState('')
  const [desiredStatus, setDesiredStatus] = useState('')

  const [deviceForm, setDeviceForm] = useState({
    deviceId: '',
    checkinIntervalSec: '',
    components: [newComponentRow()],
  })
  const [deviceFormDirty, setDeviceFormDirty] = useState(false)

  const [logsDeviceId, setLogsDeviceId] = useState('')
  const [logRows, setLogRows] = useState([])
  const [logError, setLogError] = useState('')
  const [logFilter, setLogFilter] = useState('ALL')
  const [logSort, setLogSort] = useState('desc')
  const [logFrom, setLogFrom] = useState('')
  const [logTo, setLogTo] = useState('')
  const [logLoading, setLogLoading] = useState(false)

  const [healthSummary, setHealthSummary] = useState(null)
  const [healthSummaryError, setHealthSummaryError] = useState('')
  const [healthHistory, setHealthHistory] = useState([])
  const [healthRange, setHealthRange] = useState(() => {
    return localStorage.getItem('hwops-range-dashboard') || '1h'
  })
  const [metricsSnapshot, setMetricsSnapshot] = useState({
    dbOpenConns: null,
    dbInUse: null,
    dbWaitCount: null,
    s3ObjectsTotal: null,
    s3BytesTotal: null,
    devicesTotal: null,
    pendingEnrollActive: null,
    pendingEnrollThrottleTotal: null,
    httpLatencyAvg: null,
    devicesStatus: {},
    updatedAt: '',
  })
  const [metricsHistory, setMetricsHistory] = useState([])
  const [metricsRange, setMetricsRange] = useState(() => {
    return localStorage.getItem('hwops-range-metrics') || '1h'
  })
  const [metricsError, setMetricsError] = useState('')

  const [auditRows, setAuditRows] = useState([])
  const [auditLoading, setAuditLoading] = useState(false)
  const [auditError, setAuditError] = useState('')
  const [auditAction, setAuditAction] = useState('')
  const [auditActorType, setAuditActorType] = useState('')
  const [auditActorId, setAuditActorId] = useState('')
  const [auditTargetType, setAuditTargetType] = useState('')
  const [auditTargetId, setAuditTargetId] = useState('')
  const [auditStatus, setAuditStatus] = useState('')
  const [auditFrom, setAuditFrom] = useState('')
  const [auditTo, setAuditTo] = useState('')
  const [auditLimit, setAuditLimit] = useState('200')
  const [auditRetention, setAuditRetentionState] = useState({ days: 90, updatedAt: '' })
  const [auditRetentionDays, setAuditRetentionDays] = useState('90')
  const [auditRetentionStatus, setAuditRetentionStatus] = useState('')
  const [logsTab, setLogsTab] = useState('events')
  const [rotationStatus, setRotationStatus] = useState(null)
  const [rotationLoading, setRotationLoading] = useState(false)
  const [rotationError, setRotationError] = useState('')
  const [rotationMessage, setRotationMessage] = useState('')
  const [enrollmentProfiles, setEnrollmentProfiles] = useState([])
  const [enrollmentProfilesLoading, setEnrollmentProfilesLoading] = useState(false)
  const [enrollmentProfilesError, setEnrollmentProfilesError] = useState('')
  const [enrollmentProfilesStatus, setEnrollmentProfilesStatus] = useState('')
  const [enrollmentProfileCreateOpen, setEnrollmentProfileCreateOpen] = useState(false)
  const [latestEnrollmentProfileToken, setLatestEnrollmentProfileToken] = useState('')
  const [latestEnrollmentProfileTokenName, setLatestEnrollmentProfileTokenName] = useState('')
  const [enrollmentProfileForm, setEnrollmentProfileForm] = useState({
    name: 'default-profile',
    expiresInDays: '30',
    maxUses: '1',
    challengeSecret: '',
    challengeHint: '',
    approvalDelaySec: '0',
    defaultLabelRows: [{ key: '', value: '' }],
    allowUnsignedHardwareIdentity: false,
  })
  const [editingEnrollmentProfileId, setEditingEnrollmentProfileId] = useState('')
  const [enrollmentProfileEditForm, setEnrollmentProfileEditForm] = useState({
    name: '',
    maxUses: '0',
    challengeEnabled: false,
    challengeSecret: '',
    challengeHint: '',
    approvalDelaySec: '0',
    defaultLabelRows: [{ key: '', value: '' }],
    allowUnsignedHardwareIdentity: false,
  })
  const [pendingEnrollments, setPendingEnrollments] = useState([])
  const [pendingEnrollmentsLoading, setPendingEnrollmentsLoading] = useState(false)
  const [pendingEnrollmentsError, setPendingEnrollmentsError] = useState('')
  const [pendingEnrollmentsStatus, setPendingEnrollmentsStatus] = useState('')
  const [pendingEnrollmentsFilter, setPendingEnrollmentsFilter] = useState('pending')
  const [pendingEnrollmentAlertCount, setPendingEnrollmentAlertCount] = useState(0)

  const [eventsFeed, setEventsFeed] = useState([])
  const [eventsStatus, setEventsStatus] = useState('disconnected')
  const [eventsError, setEventsError] = useState('')
  const [eventRows, setEventRows] = useState([])
  const [eventLoading, setEventLoading] = useState(false)
  const [eventQueryError, setEventQueryError] = useState('')
  const [eventType, setEventType] = useState('')
  const [eventDeviceId, setEventDeviceId] = useState('')
  const [eventFrom, setEventFrom] = useState('')
  const [eventTo, setEventTo] = useState('')
  const [eventLimit, setEventLimit] = useState('200')
  const [eventRetention, setEventRetentionState] = useState({ days: 30, updatedAt: '' })
  const [eventRetentionDays, setEventRetentionDays] = useState('30')
  const [eventRetentionStatus, setEventRetentionStatus] = useState('')
  const [maintenance, setMaintenanceState] = useState({ enabled: false, message: '', updatedAt: '' })
  const [maintenanceError, setMaintenanceError] = useState('')
  const [maintenanceStatus, setMaintenanceStatus] = useState('')
  const [upgrade, setUpgrade] = useState({ enabled: false, running: false, state: 'disabled' })
  const [upgradeError, setUpgradeError] = useState('')
  const [upgradeStatus, setUpgradeStatus] = useState('')
  const [upgradeAvailable, setUpgradeAvailable] = useState({ available: false, latest: '', bundles: [], updatesDir: '' })
  const [upgradeAvailableError, setUpgradeAvailableError] = useState('')
  const [upgradePreflight, setUpgradePreflight] = useState({ ok: false, checks: [], timestamp: '' })
  const [upgradePreflightStatus, setUpgradePreflightStatus] = useState('')
  const [upgradePreflightError, setUpgradePreflightError] = useState('')
  const [backups, setBackups] = useState([])
  const [backupStatus, setBackupStatus] = useState({ enabled: false, running: false, state: 'disabled' })
  const [restoreStatus, setRestoreStatus] = useState({ enabled: false, running: false, state: 'disabled' })
  const [backupError, setBackupError] = useState('')
  const [backupMessage, setBackupMessage] = useState('')
  const [selectedBackupId, setSelectedBackupId] = useState('')
  const selectedDeviceIdRef = useRef('')
  const refreshTimerRef = useRef(null)
  const pendingRealtimeRefreshRef = useRef({ devices: false, artifacts: false })
  const deviceOrderRef = useRef([])
  const eventsConnRef = useRef('disconnected')
  const lastEventAtRef = useRef(0)
  const eventsHeartbeatRef = useRef(null)
  const drawerResizingRef = useRef(false)
  const groupSelectionAnchorRef = useRef(-1)
  const deviceSelectionAnchorRef = useRef(-1)
  const artifactSelectionAnchorRef = useRef(-1)

  const [theme, setTheme] = useState(() => {
    return localStorage.getItem('hwops-theme') || 'dark'
  })
  const preflightOk = Boolean(upgradePreflight.ok)
  const upgradeReady = Boolean(preflightOk && upgradeAvailable.available)

  useEffect(() => {
    const groupIDs = new Set(groups.map((group) => group.groupId))
    setSelectedGroupIds((prev) => prev.filter((id) => groupIDs.has(id)))
    if (selectedGroupId && !groupIDs.has(selectedGroupId)) {
      setSelectedGroupId('')
    }
  }, [groups, selectedGroupId])

  useEffect(() => {
    const deviceIDs = new Set(devices.map((device) => device.deviceId))
    setSelectedDeviceIds((prev) => prev.filter((id) => deviceIDs.has(id)))
    if (selectedDeviceId && !deviceIDs.has(selectedDeviceId)) {
      setSelectedDeviceId('')
      setDeviceDetail(null)
      setDeviceFormDirty(false)
    }
  }, [devices, selectedDeviceId])

  useEffect(() => {
    const artifactIDs = new Set(artifacts.map((artifact) => artifact.artifactId))
    setSelectedArtifactIds((prev) => prev.filter((id) => artifactIDs.has(id)))
  }, [artifacts])

  useEffect(() => {
    if (!editingEnrollmentProfileId) return
    const exists = enrollmentProfiles.some((profile) => profile.profileId === editingEnrollmentProfileId)
    if (!exists) {
      handleCancelEditEnrollmentProfile()
    }
  }, [enrollmentProfiles, editingEnrollmentProfileId])

  function buildComponentRows(desiredComponents, legacy, current, fallbackKey = '') {
    const rows = []
    const source = desiredComponents && Object.keys(desiredComponents).length > 0 ? desiredComponents : null
    if (source) {
      Object.entries(source).forEach(([key, comp]) => {
        if (key === 'app_bundle') return
        const policy = parsePolicyObject(comp.policy)
        rows.push(newComponentRow({
          key,
          artifactType: fallbackComponentType(key, comp.artifactType),
          artifactId: comp.artifactId || '',
          desiredVersion: comp.desiredVersion || '',
          desiredConfigRev: comp.desiredConfigRev || '',
          policy,
          autoTrackMode: getAutoTrackModeFromPolicy(policy),
          locked: Boolean(comp.locked),
        }))
      })
    }
    if (rows.length === 0 && current?.components) {
      Object.entries(current.components).forEach(([key, comp]) => {
        if (key === 'app_bundle') return
        rows.push(newComponentRow({
          key,
          artifactType: fallbackComponentType(key, ''),
          desiredVersion: comp.currentVersion || '',
          desiredConfigRev: comp.currentConfigRev || '',
        }))
      })
    }
    if (rows.length === 0) {
      rows.push(newComponentRow({
        key: fallbackKey,
        artifactType: fallbackComponentType(fallbackKey, ''),
        desiredVersion: current?.softwareVersion || '',
        desiredConfigRev: current?.configRev || '',
      }))
    }
    return rows
  }

  function normalizeComponentKeyUI(value) {
    return String(value || '').trim().toLowerCase()
  }

  function buildComponentsPayload(rows) {
    const components = {}
    for (const row of rows || []) {
      const hasValues = Boolean(
        row.key ||
        row.artifactType ||
        row.artifactId ||
        row.desiredVersion ||
        row.desiredConfigRev ||
        (row.autoTrackMode && row.autoTrackMode !== 'inherit') ||
        (row.policy && Object.keys(parsePolicyObject(row.policy)).length > 0) ||
        row.locked,
      )
      if (!hasValues) continue
      const key = normalizeComponentKeyUI(row.key)
      if (!key) {
        return { error: 'Component name is required.' }
      }
      if (components[key]) {
        return { error: `Duplicate component name: ${key}` }
      }
      const artifactType = normalizeArtifactType(row.artifactType)
      if (!artifactType) {
        return { error: `Component ${key} requires an artifact type.` }
      }
      const mergedPolicy = mergeAutoTrackModeIntoPolicy(row.policy, row.autoTrackMode)
      components[key] = {
        artifactId: row.artifactId || undefined,
        artifactType,
        desiredVersion: row.desiredVersion || undefined,
        desiredConfigRev: row.desiredConfigRev || undefined,
        policy: Object.keys(mergedPolicy).length > 0 ? mergedPolicy : undefined,
        locked: row.locked,
      }
    }
    return { components }
  }

  function updateDeviceComponent(index, patch) {
    setDeviceForm((prev) => {
      const next = [...(prev.components || [])]
      if (!next[index]) return prev
      next[index] = { ...next[index], ...patch }
      return { ...prev, components: next }
    })
    setDeviceFormDirty(true)
  }

  function addDeviceComponent() {
    setDeviceForm((prev) => ({
      ...prev,
      components: [...(prev.components || []), newComponentRow()],
    }))
    setDeviceFormDirty(true)
  }

  function removeDeviceComponent(index) {
    setDeviceForm((prev) => ({
      ...prev,
      components: (prev.components || []).filter((_, idx) => idx !== index),
    }))
    setDeviceFormDirty(true)
  }

  function updateGroupComponent(index, patch) {
    setGroupDesiredForm((prev) => {
      const next = [...(prev.components || [])]
      if (!next[index]) return prev
      next[index] = { ...next[index], ...patch }
      return { ...prev, components: next }
    })
  }

  function addGroupComponent() {
    setGroupDesiredForm((prev) => ({
      ...prev,
      components: [...(prev.components || []), newComponentRow()],
    }))
  }

  function removeGroupComponent(index) {
    setGroupDesiredForm((prev) => ({
      ...prev,
      components: (prev.components || []).filter((_, idx) => idx !== index),
    }))
  }

  function updateGroupMultiDesiredComponent(index, patch) {
    setGroupMultiDesiredForm((prev) => {
      const next = [...(prev.components || [])]
      if (!next[index]) return prev
      next[index] = { ...next[index], ...patch }
      return { ...prev, components: next }
    })
  }

  function addGroupMultiDesiredComponent() {
    setGroupMultiDesiredForm((prev) => ({
      ...prev,
      components: [...(prev.components || []), newComponentRow()],
    }))
  }

  function removeGroupMultiDesiredComponent(index) {
    setGroupMultiDesiredForm((prev) => ({
      ...prev,
      components: (prev.components || []).filter((_, idx) => idx !== index),
    }))
  }

  function openArtifactPicker(scope, index) {
    if (!canManageDesiredState) return
    setArtifactPickerTarget({ scope, index })
    setArtifactModalOpen(true)
  }

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('hwops-theme', theme)
  }, [theme])

  useEffect(() => {
    localStorage.setItem('hwops-drawer-width', String(drawerWidth))
  }, [drawerWidth])

  useEffect(() => {
    localStorage.setItem('hwops-range-dashboard', healthRange)
  }, [healthRange])

  useEffect(() => {
    localStorage.setItem('hwops-range-metrics', metricsRange)
  }, [metricsRange])

  useEffect(() => {
    selectedDeviceIdRef.current = selectedDeviceId
  }, [selectedDeviceId])

  useEffect(() => {
    deviceOrderRef.current = deviceOrder
  }, [deviceOrder])

  useEffect(() => {
    if (authStatus.loaded && authStatus.enabled && !authToken) {
      return
    }
    if (authStatus.loaded && authStatus.enabled && authToken && !authUser) {
      return
    }
    loadDevices()
    loadGroups()
    loadArtifacts()
    loadArtifactLifecyclePolicy()
    loadArtifactLifecycleStatus()
    loadReleaseAutoUpdate()
    loadDesired()
    loadMaintenance()
    loadUpgrade()
    loadUpgradeAvailable()
    loadUpgradePreflight()
    loadHealthSummary()
    loadBackups()
    loadBackupStatus()
    loadRestoreStatus()
    loadAuditRetention()
  }, [authStatus.loaded, authStatus.enabled, authToken, authUser, permissions])

  useEffect(() => {
    if (!selectedDeviceId) return
    setDeviceDetail(null)
    setDeviceDetailError('')
    getDevice(selectedDeviceId)
      .then(setDeviceDetail)
      .catch((err) => setDeviceDetailError(err.message || String(err)))
  }, [selectedDeviceId])

  useEffect(() => {
    if (selectedDeviceId && !logsDeviceId) {
      setLogsDeviceId(selectedDeviceId)
    }
  }, [selectedDeviceId, logsDeviceId])

  useEffect(() => {
    if (!selectedDeviceId) return
    const desired = desiredState.devices?.find((d) => d.deviceId === selectedDeviceId)
    const current = deviceDetail?.current
    if (deviceFormDirty) return
    const desiredKeys = desired?.components ? Object.keys(desired.components) : []
    const currentKeys = current?.components ? Object.keys(current.components) : []
    const pickDefaultKey = (keys) => {
      if (keys.includes('agent_bundle')) return 'agent_bundle'
      const nonLegacy = keys.filter((key) => key !== 'app_bundle')
      return nonLegacy[0] || ''
    }
    const fallbackComponentKey = desiredKeys.length > 0
      ? pickDefaultKey(desiredKeys)
      : (currentKeys.length > 0 ? pickDefaultKey(currentKeys) : '')
    setDeviceForm({
      deviceId: selectedDeviceId,
      checkinIntervalSec: desired?.checkinIntervalSec ? String(desired.checkinIntervalSec) : '',
      components: buildComponentRows(
        desired?.components || {},
        {
          artifactId: desired?.artifactId || '',
          desiredVersion: desired?.desiredVersion || current?.softwareVersion || '',
          desiredConfigRev: desired?.desiredConfigRev || current?.configRev || '',
        },
        current,
        fallbackComponentKey,
      ),
    })
  }, [selectedDeviceId, desiredState, deviceDetail, deviceFormDirty])

  useEffect(() => {
    if (!selectedGroupId) return
    const desired = desiredState.groups?.find((g) => g.groupId === selectedGroupId)
    setGroupDesiredForm({
      groupId: selectedGroupId,
      checkinIntervalSec: desired?.checkinIntervalSec ? String(desired.checkinIntervalSec) : '',
      components: buildComponentRows(
        desired?.components || {},
        {
          artifactId: desired?.artifactId || '',
          desiredVersion: desired?.desiredVersion || '',
          desiredConfigRev: desired?.desiredConfigRev || '',
        },
        null,
      ),
    })
  }, [selectedGroupId, desiredState])

  useEffect(() => {
    if (!upgrade.running) return
    const timer = setInterval(() => {
      loadUpgrade()
    }, 5000)
    return () => clearInterval(timer)
  }, [upgrade.running])

  useEffect(() => {
    if (!backupStatus.running) return
    const timer = setInterval(() => {
      loadBackupStatus()
      loadBackups()
    }, 5000)
    return () => clearInterval(timer)
  }, [backupStatus.running])

  useEffect(() => {
    if (!restoreStatus.running) return
    const timer = setInterval(() => {
      loadRestoreStatus()
    }, 5000)
    return () => clearInterval(timer)
  }, [restoreStatus.running])

  const canViewMetrics = permissions.canViewMetrics
  const canViewAudit = permissions.canViewAudit
  const canManageUsers = permissions.canManageUsers
  const canManageArtifacts = permissions.canManageArtifacts
  const canManageArtifactLifecycle = permissions.canManageArtifactLifecycle
  const canManageDesiredState = permissions.canManageDesiredState
  const canManageGroups = permissions.canManageGroups
  const canEditDeviceLabels = permissions.canEditDeviceLabels
  const canDecommissionDevices = permissions.canDecommissionDevices
  const canRotate = permissions.canRotateCertificates
  const canManagePendingEnrollments = permissions.canManagePendingEnrollments
  const canManageMaintenance = permissions.canManageMaintenance
  const canViewBackups = permissions.canViewBackups
  const canManageBackups = permissions.canManageBackups
  const canManageReleaseAutoUpdate = permissions.canManageReleaseAutoUpdate
  const canManageAuditRetention = permissions.canManageAuditRetention
  const canManageEventRetention = permissions.canManageEventRetention
  const canApplyUpgrade = permissions.canApplyUpgrade

  useEffect(() => {
    if (permissions.logsTabs[logsTab]) return
    setLogsTab(fallbackLogsTab)
  }, [permissions, logsTab, fallbackLogsTab])

  useEffect(() => {
    if (canManageArtifacts) return
    setArtifactUploadOpen(false)
  }, [canManageArtifacts])

  useEffect(() => {
    if (canManageGroups) return
    setGroupModalOpen(false)
    setGroupMultiEditOpen(false)
    setGroupBulkOpen(false)
  }, [canManageGroups])

  useEffect(() => {
    if (canManageDesiredState) return
    setGroupDesiredOpen(false)
    setGroupMultiDesiredOpen(false)
  }, [canManageDesiredState])

  useEffect(() => {
    if (canManagePendingEnrollments) return
    setEnrollmentProfileCreateOpen(false)
    setEditingEnrollmentProfileId('')
  }, [canManagePendingEnrollments])

  useEffect(() => {
    if (view !== 'dashboard') return undefined
    loadHealthSummary()
    const timer = setInterval(() => {
      loadHealthSummary()
    }, 15000)
    return () => clearInterval(timer)
  }, [view])

  useEffect(() => {
    if (view !== 'metrics') return undefined
    loadMetrics()
    const timer = setInterval(() => {
      loadMetrics()
    }, 15000)
    return () => clearInterval(timer)
  }, [view, canViewMetrics])

  useEffect(() => {
    if (view !== 'security') return undefined
    loadRotationStatus()
    loadEnrollmentProfiles()
    loadPendingEnrollments()
    const timer = setInterval(() => {
      loadRotationStatus()
      loadEnrollmentProfiles({ silent: true })
      loadPendingEnrollments({ silent: true })
    }, 15000)
    return () => clearInterval(timer)
  }, [view])

  useEffect(() => {
    if (view !== 'security') return
    loadPendingEnrollments()
  }, [view, pendingEnrollmentsFilter, canManagePendingEnrollments])

  useEffect(() => {
    if (!canManagePendingEnrollments) {
      setPendingEnrollmentAlertCount(0)
      return undefined
    }
    const load = () => {
      loadPendingEnrollmentAlertCount().catch(() => {})
    }
    load()
    const timer = setInterval(load, 15000)
    return () => clearInterval(timer)
  }, [canManagePendingEnrollments])

  useEffect(() => {
    const latest = eventsFeed[0]
    if (!canManagePendingEnrollments || !latest || latest.type !== 'device.enroll_pending') return
    loadPendingEnrollmentAlertCount().catch(() => {})
  }, [eventsFeed, canManagePendingEnrollments])

  useEffect(() => {
    if (view !== 'settings') return
    loadReleaseAutoUpdate()
  }, [view])

  useEffect(() => {
    if (view !== 'logs' || logsTab !== 'events') return
    loadEventsHistory()
    if (canManageEventRetention) {
      loadEventRetention()
    }
  }, [view, logsTab, canManageEventRetention])

  useEffect(() => {
    if (!deviceDrawerOpen) return
    const viewport = window.innerWidth
    const autoWidth = Math.min(Math.max(560, viewport * 0.6), viewport * 0.9)
    if (drawerWidth < autoWidth) {
      setDrawerWidth(autoWidth)
    }
  }, [deviceDrawerOpen, drawerWidth])

  useEffect(() => {
    if (!deviceDrawerOpen) return undefined
    const handlePointerMove = (event) => {
      if (!drawerResizingRef.current) return
      const viewport = window.innerWidth
      const next = Math.min(Math.max(viewport - event.clientX, 320), viewport * 0.9)
      setDrawerWidth(next)
    }
    const handlePointerUp = () => {
      drawerResizingRef.current = false
    }
    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', handlePointerUp)
    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
    }
  }, [deviceDrawerOpen])

  useEffect(() => {
    if (authStatus.loaded && authStatus.enabled && !authToken) {
      setEventsStatus('disconnected')
      return
    }
    let ws
    let reconnectTimer
    let shouldReconnect = true
    const staleMs = 30_000

    const connect = () => {
      eventsConnRef.current = 'connecting'
      setEventsStatus('connecting')
      setEventsError('')
      ws = new WebSocket(wsUrl)
      ws.onopen = () => {
        eventsConnRef.current = 'connected'
        setEventsStatus('connected')
      }
      ws.onmessage = (evt) => {
        try {
          const data = JSON.parse(evt.data)
          const eventType = typeof data?.type === 'string' ? data.type : ''
          const isDeviceEvent = eventType.startsWith('device.')
          const isArtifactEvent = eventType.startsWith('artifact.')
          if (isDeviceEvent) {
            pendingRealtimeRefreshRef.current.devices = true
          }
          if (isArtifactEvent) {
            pendingRealtimeRefreshRef.current.artifacts = true
          }
          setEventsFeed((prev) => [data, ...prev].slice(0, 200))
          lastEventAtRef.current = Date.now()
          if (!refreshTimerRef.current) {
            refreshTimerRef.current = setTimeout(() => {
              refreshTimerRef.current = null
              const pending = pendingRealtimeRefreshRef.current
              pendingRealtimeRefreshRef.current = { devices: false, artifacts: false }

              if (pending.devices) {
                loadDevices({ silent: true })
                const currentId = selectedDeviceIdRef.current
                if (currentId) {
                  getDevice(currentId)
                    .then(setDeviceDetail)
                    .catch((err) => setDeviceDetailError(err.message || String(err)))
                }
              }
              if (pending.artifacts) {
                loadArtifacts()
              }
            }, 500)
          }
        } catch (err) {
          setEventsError('Failed to parse event payload')
        }
      }
      ws.onerror = () => {
        eventsConnRef.current = 'disconnected'
        setEventsStatus('disconnected')
      }
      ws.onclose = () => {
        eventsConnRef.current = 'disconnected'
        setEventsStatus('disconnected')
        if (shouldReconnect) {
          reconnectTimer = setTimeout(connect, 2000)
        }
      }
    }

    connect()
    eventsHeartbeatRef.current = setInterval(() => {
      const now = Date.now()
      const last = lastEventAtRef.current
      if (!last || now-last > staleMs) {
        if (eventsConnRef.current === 'connected') {
          setEventsStatus('stale')
        } else {
          setEventsStatus('disconnected')
        }
      } else {
        setEventsStatus('connected')
      }
    }, 5000)
    return () => {
      shouldReconnect = false
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (refreshTimerRef.current) {
        clearTimeout(refreshTimerRef.current)
        refreshTimerRef.current = null
      }
      if (eventsHeartbeatRef.current) {
        clearInterval(eventsHeartbeatRef.current)
        eventsHeartbeatRef.current = null
      }
      if (ws) ws.close()
    }
  }, [wsUrl])

  const dashboard = useMemo(() => {
    const summary = healthSummary?.devices
    const total = summary?.total ?? devices.length
    const active = summary?.active ?? devices.filter((d) => d.status === 'active').length
    const stale = summary?.stale ?? devices.filter((d) => d.status === 'stale').length
    const offline = summary?.offline ?? devices.filter((d) => d.status === 'offline').length
    const degraded = summary?.degraded ?? devices.filter((d) => d.status === 'degraded').length
    const lastSeen = summary?.lastSeen
      ? new Date(summary.lastSeen)
      : devices
        .map((d) => (d.lastSeen ? new Date(d.lastSeen) : null))
        .filter(Boolean)
        .sort((a, b) => b - a)[0]
    const lastSeenLabel = lastSeen ? lastSeen.toISOString() : '—'
    return { total, active, stale, offline, degraded, lastSeenLabel }
  }, [healthSummary, devices, deviceDetail])

  const preApplyBadgeClass = useMemo(() => {
    const raw = (deviceDetail?.current?.lastPreApplyStatus || '').toLowerCase()
    if (!raw || raw === '—') return 'unknown'
    return raw
  }, [deviceDetail])

  const notifications = useMemo(() => {
    const items = []
    if (devicesError) items.push({ type: 'error', text: devicesError })
    if (groupsError) items.push({ type: 'error', text: groupsError })
    if (artifactsError) items.push({ type: 'error', text: artifactsError })
    if (desiredError) items.push({ type: 'error', text: desiredError })
    if (eventsError) items.push({ type: 'error', text: eventsError })
    if (eventQueryError) items.push({ type: 'error', text: eventQueryError })
    if (maintenanceError) items.push({ type: 'error', text: maintenanceError })
    if (upgradeError) items.push({ type: 'error', text: upgradeError })
    if (upgradeAvailableError) items.push({ type: 'error', text: upgradeAvailableError })
    if (uploadStatus) items.push({ type: 'info', text: uploadStatus })
    if (devicesStatus) items.push({ type: 'info', text: devicesStatus })
    if (groupsStatus) items.push({ type: 'info', text: groupsStatus })
    if (artifactsStatus) items.push({ type: 'info', text: artifactsStatus })
    if (desiredStatus) items.push({ type: 'info', text: desiredStatus })
    if (maintenanceStatus) items.push({ type: 'info', text: maintenanceStatus })
    if (upgradeStatus) items.push({ type: 'info', text: upgradeStatus })
    if (canManagePendingEnrollments && pendingEnrollmentAlertCount > 0) {
      items.unshift({
        type: 'warning',
        text: `${pendingEnrollmentAlertCount} first-contact enrollment${pendingEnrollmentAlertCount === 1 ? '' : 's'} pending approval`,
        actionLabel: 'Review',
        onAction: () => {
          setPendingEnrollmentsFilter('pending')
          setView('security')
        },
      })
    }
    return items.slice(0, 4)
  }, [devicesError, groupsError, artifactsError, desiredError, eventsError, eventQueryError, maintenanceError, upgradeError, upgradeAvailableError, uploadStatus, devicesStatus, groupsStatus, artifactsStatus, desiredStatus, maintenanceStatus, upgradeStatus, canManagePendingEnrollments, pendingEnrollmentAlertCount])

  async function doLogin() {
    setLoginStatus('Signing in...')
    setAuthError('')
    try {
      const res = await apiLogin(loginForm.email, loginForm.password)
      if (!res?.token) {
        throw new Error('No token returned')
      }
      persistAuthToken(res.token)
      setAuthToken(res.token)
      setLoginStatus('Signed in')
      setLoginForm((prev) => ({ ...prev, password: '' }))
      setAuthUser(res.user || null)
      setAuthUserLoaded(true)
      setAuthView('login')
      loadUsers()
    } catch (err) {
      setLoginStatus('')
      setAuthError(err.message || String(err))
    }
  }

  async function doRegister() {
    setRegisterStatus('Creating account...')
    setAuthError('')
    try {
      await registerWithVoucher({
        token: registerForm.token,
        email: registerForm.email,
        password: registerForm.password,
        displayName: registerForm.displayName,
      })
      setRegisterStatus('Account created, signing in...')
      const res = await apiLogin(registerForm.email, registerForm.password)
      persistAuthToken(res.token)
      setAuthToken(res.token)
      setAuthUser(res.user || null)
      setAuthUserLoaded(true)
      setRegisterForm({ token: '', email: '', password: '', displayName: '' })
      setRegisterStatus('')
      setAuthView('login')
    } catch (err) {
      setRegisterStatus('')
      setAuthError(err.message || String(err))
    }
  }

  async function doDownloadBootstrapCA() {
    setBootstrapStatusMessage('Downloading CA certificate...')
    try {
      const blob = await downloadBootstrapCA(bootstrapToken.trim() || undefined)
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'hardwareops-ca.crt'
      document.body.appendChild(a)
      a.click()
      a.remove()
      window.URL.revokeObjectURL(url)
      setBootstrapStatusMessage('Downloaded hardwareops-ca.crt')
    } catch (err) {
      setBootstrapStatusMessage(err.message || String(err))
    }
  }

  function doLogout() {
    persistAuthToken('')
    setAuthToken('')
    setAuthUser(null)
    setAuthUserLoaded(true)
  }

  async function loadUsers() {
    if (!canManageUsers) {
      setUsers([])
      setUsersStatus('')
      setUsersError('')
      return
    }
    setUsersStatus('Loading users...')
    setUsersError('')
    try {
      const res = await listUsers()
      setUsers(res.items || [])
      setUsersStatus('')
    } catch (err) {
      setUsersStatus('')
      setUsersError(err.message || String(err))
    }
  }

  async function submitUser() {
    if (!canManageUsers) return
    setUsersStatus('Creating user...')
    setUsersError('')
    try {
      await createUser({
        email: userForm.email,
        password: userForm.password,
        displayName: userForm.displayName,
        roles: userForm.roles,
      })
      setUsersStatus('User created')
      setUserForm({ email: '', password: '', displayName: '', roles: ['viewer'] })
      await loadUsers()
    } catch (err) {
      setUsersStatus('')
      setUsersError(err.message || String(err))
    }
  }

  function toggleRole(role) {
    setUserForm((prev) => {
      const roles = new Set(prev.roles)
      if (roles.has(role)) {
        roles.delete(role)
      } else {
        roles.add(role)
      }
      if (roles.size === 0) {
        roles.add('viewer')
      }
      return { ...prev, roles: Array.from(roles) }
    })
  }

  function toggleVoucherRole(role) {
    setVoucherForm((prev) => {
      const roles = new Set(prev.roles)
      if (roles.has(role)) {
        roles.delete(role)
      } else {
        roles.add(role)
      }
      if (roles.size === 0) {
        roles.add('viewer')
      }
      return { ...prev, roles: Array.from(roles) }
    })
  }

  async function submitVoucher() {
    if (!canManageUsers) return
    setVoucherStatus('Creating voucher...')
    setVoucherToken('')
    setUsersError('')
    try {
      const res = await createVoucher({
        email: voucherForm.email,
        roles: voucherForm.roles,
        ttlHours: Number(voucherForm.ttlHours || '24'),
      })
      setVoucherToken(res.token || '')
      setVoucherStatus('Voucher created')
      setVoucherForm({ email: '', ttlHours: '24', roles: ['viewer'] })
    } catch (err) {
      setVoucherStatus('')
      setUsersError(err.message || String(err))
    }
  }

  function loadDevices(options = {}) {
    const { silent = false } = options
    if (!silent) {
      setDevicesLoading(true)
    }
    setDevicesError('')
    getDevices()
      .then((res) => {
        const items = res.items || []
        const prevOrder = deviceOrderRef.current || []
        const itemIds = items.map((item) => item.deviceId)
        const nextOrder = prevOrder.filter((id) => itemIds.includes(id))
        itemIds.forEach((id) => {
          if (!nextOrder.includes(id)) {
            nextOrder.push(id)
          }
        })
        setDeviceOrder(nextOrder)
        const orderIndex = new Map(nextOrder.map((id, idx) => [id, idx]))
        const sorted = [...items].sort((a, b) => {
          const ai = orderIndex.get(a.deviceId) ?? 0
          const bi = orderIndex.get(b.deviceId) ?? 0
          return ai - bi
        })
        setDevices(sorted)
        const currentSelected = selectedDeviceIdRef.current
        if (!currentSelected && sorted.length > 0) {
          setSelectedDeviceId(sorted[0].deviceId)
        } else if (currentSelected && !sorted.some((item) => item.deviceId === currentSelected)) {
          setSelectedDeviceId(sorted[0]?.deviceId || '')
        }
      })
      .catch((err) => setDevicesError(err.message || String(err)))
      .finally(() => {
        if (!silent) {
          setDevicesLoading(false)
        }
      })
  }

  function loadGroups() {
    setGroupsError('')
    listGroups()
      .then((res) => setGroups(res.items || []))
      .catch((err) => setGroupsError(err.message || String(err)))
  }

  function loadArtifacts() {
    setArtifactsError('')
    listArtifacts()
      .then((res) => setArtifacts(res.items || []))
      .catch((err) => setArtifactsError(err.message || String(err)))
  }

  function loadArtifactLifecyclePolicy() {
    getArtifactLifecyclePolicy()
      .then((res) => {
        const days = Number(res?.deprecatedDeleteAfterDays || 30)
        setArtifactLifecyclePolicy({
          deprecatedDeleteAfterDays: days,
          updatedAt: res?.updatedAt || '',
        })
        setArtifactLifecyclePolicyInput(String(days))
      })
      .catch((err) => setArtifactsError(err.message || String(err)))
  }

  function loadArtifactLifecycleStatus() {
    getArtifactLifecycleStatus()
      .then((res) => {
        setArtifactLifecycleStatus({
          enabled: Boolean(res?.enabled),
          running: Boolean(res?.running),
          intervalSeconds: Number(res?.intervalSeconds || 0),
          batchLimit: Number(res?.batchLimit || 200),
          alertReferenceThreshold: Number(res?.alertReferenceThreshold || 10),
          lastRun: res?.lastRun || null,
          alerts: Array.isArray(res?.alerts) ? res.alerts : [],
        })
      })
      .catch((err) => setArtifactsError(err.message || String(err)))
  }

  function loadReleaseAutoUpdate() {
    getReleaseAutoUpdateStatus()
      .then((res) => {
        setReleaseAutoUpdate((prev) => ({
          ...prev,
          enabled: Boolean(res?.enabled),
          allowUnsigned: Boolean(res?.allowUnsigned),
          intervalSeconds: Number(res?.intervalSeconds || prev.intervalSeconds || 0),
          running: Boolean(res?.running),
          updatedAt: res?.updatedAt || '',
          updatedByUserId: res?.updatedByUserId || '',
          lastRun: res?.lastRun || null,
        }))
      })
      .catch((err) => setReleaseAutoUpdateStatus(err.message || String(err)))
  }

  function loadDesired() {
    setDesiredError('')
    getDesiredState()
      .then((res) => setDesiredState(res || { groups: [], devices: [] }))
      .catch((err) => setDesiredError(err.message || String(err)))
  }

  function handleUpload(e) {
    e.preventDefault()
    if (!canManageArtifacts) return
    const form = e.currentTarget
    const formData = new FormData(form)
    setUploadStatus('Uploading...')
    uploadArtifact(formData)
      .then((resp) => {
        setUploadStatus(`Uploaded artifact ${resp.artifactId}`)
        form.reset()
        loadArtifacts()
        setArtifactUploadOpen(false)
      })
      .catch((err) => setUploadStatus(err.message || String(err)))
  }

  function handleDesiredDevice(e) {
    e.preventDefault()
    if (!canManageDesiredState) return
    setDesiredStatus('Setting desired state...')
    const { components, error } = buildComponentsPayload(deviceForm.components)
    if (error) {
      setDesiredStatus(error)
      return
    }
    const payload = {
      components,
      checkinIntervalSec: deviceForm.checkinIntervalSec
        ? Number(deviceForm.checkinIntervalSec)
        : undefined,
    }
    setDesiredStateDevice(deviceForm.deviceId, payload)
      .then(() => {
        setDesiredStatus('Desired state set for device')
        loadDesired()
        setDeviceFormDirty(false)
      })
      .catch((err) => setDesiredStatus(err.message || String(err)))
  }

  function promptDecommissionReason(message) {
    const reasonInput = window.prompt(message, '')
    if (reasonInput === null) return null
    const reason = reasonInput.trim()
    if (!reason) {
      setDevicesError('Decommission reason is required')
      return null
    }
    return reason
  }

  async function decommissionDeviceIDs(deviceIDs, reason, statusLabel = 'Decommissioning devices') {
    if (!canDecommissionDevices) {
      setDevicesError('Admin role required to decommission devices')
      return { succeeded: 0, failed: 0, failedIDs: deviceIDs || [] }
    }
    const targets = Array.from(new Set(deviceIDs.filter(Boolean)))
    if (targets.length === 0) return { succeeded: 0, failed: 0, failedIDs: [] }

    setDevicesError('')
    setDevicesStatus(`${statusLabel}...`)

    let succeeded = 0
    let failed = 0
    let firstError = ''
    const failedIDs = []

    for (const deviceID of targets) {
      try {
        await decommissionDevice(deviceID, { reason })
        succeeded += 1
      } catch (err) {
        failed += 1
        failedIDs.push(deviceID)
        if (!firstError) {
          firstError = `${deviceID}: ${err.message || String(err)}`
        }
      }
    }

    if (firstError) {
      setDevicesError(firstError)
    }

    if (targets.includes(selectedDeviceId) && !failedIDs.includes(selectedDeviceId)) {
      setSelectedDeviceId('')
      setDeviceDetail(null)
    }

    setSelectedDeviceIds(failedIDs)
    setDevicesStatus(`${statusLabel}: ${succeeded} succeeded, ${failed} failed.`)
    loadDevices()
    loadDesired()
    return { succeeded, failed, failedIDs }
  }

  async function handleDeleteDevice(deviceId) {
    if (!canDecommissionDevices) {
      setDevicesError('Admin role required to decommission devices')
      return
    }
    const reason = promptDecommissionReason(
      `Decommission device ${deviceId}.\nThis releases a license slot and removes desired state/apply history.\nEnter reason:`,
    )
    if (!reason) return
    await decommissionDeviceIDs([deviceId], reason, `Decommissioning ${deviceId}`)
  }

  function clearDeviceSelection() {
    setSelectedDeviceIds([])
    deviceSelectionAnchorRef.current = -1
  }

  function handleDeviceRowSelect(index, checked, shiftKey) {
    setSelectedDeviceIds((prev) => {
      const next = new Set(prev)
      const anchor = deviceSelectionAnchorRef.current
      const hasRange = shiftKey && anchor >= 0 && anchor < filteredDevices.length
      if (hasRange) {
        const start = Math.min(anchor, index)
        const end = Math.max(anchor, index)
        for (let i = start; i <= end; i += 1) {
          const id = filteredDevices[i]?.deviceId
          if (!id) continue
          if (checked) next.add(id)
          else next.delete(id)
        }
      } else {
        const id = filteredDevices[index]?.deviceId
        if (id) {
          if (checked) next.add(id)
          else next.delete(id)
        }
      }
      deviceSelectionAnchorRef.current = index
      return devices.map((device) => device.deviceId).filter((id) => next.has(id))
    })
  }

  function handleSelectAllDevices(checked) {
    if (checked) {
      setSelectedDeviceIds(filteredDevices.map((device) => device.deviceId))
      return
    }
    clearDeviceSelection()
  }

  async function handleBulkDecommissionSelectedDevices() {
    if (selectedDeviceIds.length === 0) return
    if (!canDecommissionDevices) {
      setDevicesError('Admin role required to decommission devices')
      return
    }
    const confirmed = window.confirm(
      `Decommission ${selectedDeviceIds.length} selected device(s)? This releases license slots and removes desired-state/apply history.`,
    )
    if (!confirmed) return
    const reason = promptDecommissionReason('Enter decommission reason for selected devices:')
    if (!reason) return
    await decommissionDeviceIDs(selectedDeviceIds, reason, 'Bulk decommission')
  }

  async function handleDeleteArtifact(artifactId) {
    if (!canManageArtifacts) return
    const ok = window.confirm(`Delete artifact ${artifactId}?`)
    if (!ok) return
    setArtifactsStatus('Deleting artifact...')
    try {
      await deleteArtifact(artifactId)
      setArtifactsStatus(`Deleted artifact ${artifactId}`)
      loadArtifacts()
      loadDesired()
    } catch (err) {
      setArtifactsError(err.message || String(err))
    }
  }

  function clearArtifactSelection() {
    setSelectedArtifactIds([])
    artifactSelectionAnchorRef.current = -1
  }

  function handleArtifactRowSelect(index, checked, shiftKey) {
    setSelectedArtifactIds((prev) => {
      const next = new Set(prev)
      const anchor = artifactSelectionAnchorRef.current
      const hasRange = shiftKey && anchor >= 0 && anchor < visibleArtifactRows.length
      if (hasRange) {
        const start = Math.min(anchor, index)
        const end = Math.max(anchor, index)
        for (let i = start; i <= end; i += 1) {
          const id = visibleArtifactRows[i]
          if (!id) continue
          if (checked) next.add(id)
          else next.delete(id)
        }
      } else {
        const id = visibleArtifactRows[index]
        if (id) {
          if (checked) next.add(id)
          else next.delete(id)
        }
      }
      artifactSelectionAnchorRef.current = index
      return artifacts.map((artifact) => artifact.artifactId).filter((id) => next.has(id))
    })
  }

  function handleSelectAllArtifacts(checked) {
    if (checked) {
      setSelectedArtifactIds(visibleArtifactRows)
      return
    }
    clearArtifactSelection()
  }

  async function handleBulkDeleteSelectedArtifacts() {
    if (!canManageArtifacts) return
    if (selectedArtifactIds.length === 0) return
    const selectedRows = selectedArtifactIds
      .map((artifactId) => artifactByID[artifactId])
      .filter(Boolean)
    const eligible = selectedRows.filter((artifact) => {
      const lifecycle = normalizeArtifactStatus(artifact.status)
      const refs = Number(artifact.referenceCount || 0)
      return lifecycle === 'deprecated' && refs <= 0
    })
    const skipped = selectedRows.length - eligible.length
    if (eligible.length === 0) {
      setArtifactsStatus('No selected artifacts are eligible for delete (must be deprecated with zero refs).')
      return
    }
    const confirmed = window.confirm(
      `Delete ${eligible.length} selected artifact(s)? ${skipped > 0 ? `${skipped} selected artifact(s) will be skipped.` : ''}`,
    )
    if (!confirmed) return

    setArtifactsError('')
    setArtifactsStatus('Deleting selected artifacts...')
    let deleted = 0
    let failed = 0
    let firstError = ''
    const failedIDs = []
    for (const artifact of eligible) {
      try {
        await deleteArtifact(artifact.artifactId)
        deleted += 1
      } catch (err) {
        failed += 1
        failedIDs.push(artifact.artifactId)
        if (!firstError) {
          firstError = `${artifact.artifactId}: ${err.message || String(err)}`
        }
      }
    }
    if (firstError) {
      setArtifactsError(firstError)
    }
    setSelectedArtifactIds(failedIDs)
    setArtifactsStatus(
      `Bulk delete: ${deleted} deleted, ${failed} failed${skipped > 0 ? `, ${skipped} skipped` : ''}.`,
    )
    loadArtifacts()
    loadDesired()
  }

  async function handleBulkDeprecateSelectedArtifacts() {
    if (!canManageArtifacts) return
    if (selectedArtifactIds.length === 0) return
    const selectedRows = selectedArtifactIds
      .map((artifactId) => artifactByID[artifactId])
      .filter(Boolean)
    const eligible = selectedRows.filter((artifact) => normalizeArtifactStatus(artifact.status) !== 'deprecated')
    const skippedRows = selectedRows.filter((artifact) => normalizeArtifactStatus(artifact.status) === 'deprecated')
    const skipped = skippedRows.length
    if (eligible.length === 0) {
      setArtifactsStatus('No selected artifacts are eligible for deprecate (already deprecated).')
      return
    }
    const confirmed = window.confirm(
      `Deprecate ${eligible.length} selected artifact(s)? ${skipped > 0 ? `${skipped} selected artifact(s) will be skipped.` : ''}`,
    )
    if (!confirmed) return

    setArtifactsError('')
    setArtifactsStatus('Deprecating selected artifacts...')
    const retentionDays = Number(artifactLifecyclePolicyInput)
    const retention = Number.isFinite(retentionDays) && retentionDays > 0 ? retentionDays : undefined
    let deprecated = 0
    let failed = 0
    let firstError = ''
    const failedIDs = []
    for (const artifact of eligible) {
      try {
        await deprecateArtifact(artifact.artifactId, retention)
        deprecated += 1
      } catch (err) {
        failed += 1
        failedIDs.push(artifact.artifactId)
        if (!firstError) {
          firstError = `${artifact.artifactId}: ${err.message || String(err)}`
        }
      }
    }
    if (firstError) {
      setArtifactsError(firstError)
    }
    const skippedIDs = skippedRows.map((artifact) => artifact.artifactId)
    setSelectedArtifactIds(Array.from(new Set([...failedIDs, ...skippedIDs])))
    setArtifactsStatus(
      `Bulk deprecate: ${deprecated} deprecated, ${failed} failed${skipped > 0 ? `, ${skipped} skipped` : ''}.`,
    )
    loadArtifacts()
  }

  async function handleDeprecateArtifact(artifactId) {
    if (!canManageArtifacts) return
    const ok = window.confirm(`Deprecate artifact ${artifactId}?`)
    if (!ok) return
    setArtifactsStatus('Deprecating artifact...')
    try {
      await deprecateArtifact(artifactId, Number(artifactLifecyclePolicyInput) || undefined)
      setArtifactsStatus(`Deprecated artifact ${artifactId}`)
      loadArtifacts()
    } catch (err) {
      setArtifactsError(err.message || String(err))
    }
  }

  async function handleRestoreArtifact(artifactId) {
    if (!canManageArtifacts) return
    const ok = window.confirm(`Restore artifact ${artifactId} to active state?`)
    if (!ok) return
    setArtifactsStatus('Restoring artifact...')
    try {
      await restoreArtifact(artifactId)
      setArtifactsStatus(`Restored artifact ${artifactId}`)
      loadArtifacts()
    } catch (err) {
      setArtifactsError(err.message || String(err))
    }
  }

  async function handleSaveArtifactLifecyclePolicy() {
    if (!canManageArtifactLifecycle) return
    const days = Number(artifactLifecyclePolicyInput)
    if (!Number.isFinite(days) || days < 1) {
      setArtifactsError('Retention days must be >= 1.')
      return
    }
    setArtifactsStatus('Saving lifecycle policy...')
    try {
      const res = await setArtifactLifecyclePolicy(days)
      setArtifactLifecyclePolicy({
        deprecatedDeleteAfterDays: Number(res?.deprecatedDeleteAfterDays || days),
        updatedAt: res?.updatedAt || '',
      })
      setArtifactsStatus('Lifecycle policy updated')
      loadArtifactLifecycleStatus()
    } catch (err) {
      setArtifactsError(err.message || String(err))
    }
  }

  async function handleSaveReleaseAutoUpdateSettings() {
    if (!canManageReleaseAutoUpdate) return
    setReleaseAutoUpdateSaving(true)
    setReleaseAutoUpdateStatus('Saving release auto-update settings...')
    try {
      const res = await setReleaseAutoUpdateSettings({
        enabled: Boolean(releaseAutoUpdate.enabled),
        allowUnsigned: Boolean(releaseAutoUpdate.allowUnsigned),
      })
      setReleaseAutoUpdate((prev) => ({
        ...prev,
        enabled: Boolean(res?.enabled),
        allowUnsigned: Boolean(res?.allowUnsigned),
        intervalSeconds: Number(res?.intervalSeconds || prev.intervalSeconds || 0),
        running: Boolean(res?.running),
        updatedAt: res?.updatedAt || prev.updatedAt || '',
        updatedByUserId: res?.updatedByUserId || '',
        lastRun: res?.lastRun || prev.lastRun || null,
      }))
      setReleaseAutoUpdateStatus('Release auto-update settings saved')
      loadReleaseAutoUpdate()
    } catch (err) {
      setReleaseAutoUpdateStatus(err.message || String(err))
    } finally {
      setReleaseAutoUpdateSaving(false)
    }
  }

  async function handleRunReleaseAutoUpdate() {
    if (!canManageReleaseAutoUpdate) return
    setReleaseAutoUpdateSaving(true)
    setReleaseAutoUpdateStatus('Running release auto-update...')
    try {
      const run = await runReleaseAutoUpdate()
      setReleaseAutoUpdate((prev) => ({
        ...prev,
        running: false,
        lastRun: run || null,
      }))
      setReleaseAutoUpdateStatus(
        `Run complete: updated ${Number(run?.componentsUpdated || 0)} component(s) across ${Number(run?.groupsUpdated || 0)} group(s) and ${Number(run?.devicesUpdated || 0)} device override(s).`,
      )
      loadDesired()
      loadReleaseAutoUpdate()
    } catch (err) {
      setReleaseAutoUpdateStatus(err.message || String(err))
    } finally {
      setReleaseAutoUpdateSaving(false)
    }
  }

  async function handlePruneArtifacts() {
    if (!canManageArtifactLifecycle) return
    const ok = window.confirm('Prune deprecated artifacts that reached delete-after and are no longer referenced?')
    if (!ok) return
    setArtifactsStatus('Pruning deprecated artifacts...')
    try {
      const res = await pruneArtifacts(200)
      setArtifactsStatus(`Prune complete: deleted ${res.deletedNum || 0}, skipped ${res.skippedNum || 0}`)
      loadArtifacts()
      loadDesired()
      loadArtifactLifecycleStatus()
    } catch (err) {
      setArtifactsError(err.message || String(err))
    }
  }

  async function handleSaveGroup(e) {
    e.preventDefault()
    if (!canManageGroups) return
    setGroupsStatus('')
    setGroupsError('')
    const selector = {}
    if (groupForm.region) selector.region = groupForm.region
    if (groupForm.role) selector.role = groupForm.role
    if (groupForm.site) selector.site = groupForm.site
    const invalid = groupForm.custom.some(
      (row) => (row.key && !row.value) || (!row.key && row.value),
    )
    if (invalid) {
      setGroupsError('Custom labels require both key and value.')
      return
    }
    groupForm.custom
      .filter((row) => row.key && row.value)
      .forEach((row) => {
        selector[row.key] = row.value
      })
    const groupId = groupForm.groupId || crypto.randomUUID()
    setGroupsStatus(groupForm.groupId ? 'Updating group...' : 'Creating group...')
    try {
      await putGroup(groupId, { name: groupForm.name, selector })
      setGroupsStatus('Group saved')
      setGroupForm({ groupId: '', name: '', region: '', role: '', site: '', custom: [] })
      setGroupModalOpen(false)
      loadGroups()
    } catch (err) {
      setGroupsError(err.message || String(err))
    }
  }

  async function handleDeleteGroup(groupId, forceCascade = false) {
    if (!canManageGroups) return
    if (forceCascade && !canDecommissionDevices) return
    const ok = window.confirm(`Delete group ${groupId}? This removes desired state for the group.`)
    if (!ok) return
    const cascadeDeviceIDs = listDevicesMatchingGroups([groupId])
    let cascade = forceCascade
    if (cascadeDeviceIDs.length > 0 && canDecommissionDevices) {
      if (!forceCascade) {
        cascade = window.confirm(
          `Also decommission ${cascadeDeviceIDs.length} matching device(s)?\n\nOK = delete group + decommission devices\nCancel = delete group only`,
        )
      }
    } else {
      cascade = false
    }
    setGroupsStatus('Deleting group...')
    try {
      await deleteGroup(groupId)
      setGroupsStatus(`Deleted group ${groupId}`)
      if (selectedGroupId === groupId) {
        setSelectedGroupId('')
      }
      setSelectedGroupIds((prev) => prev.filter((id) => id !== groupId))
      loadGroups()
      loadDesired()
      if (cascade && cascadeDeviceIDs.length > 0) {
        const reason = promptDecommissionReason(
          `Enter decommission reason for ${cascadeDeviceIDs.length} device(s) matched by the deleted group:`,
        )
        if (reason) {
          await decommissionDeviceIDs(cascadeDeviceIDs, reason, 'Group cascade decommission')
        }
      }
    } catch (err) {
      setGroupsError(err.message || String(err))
    }
  }

  async function applyGroupBatchActions(actions, statusPrefix) {
    if (!canManageGroups) {
      setGroupBatchError('Operator role required to manage groups.')
      return { applied: 0, failed: Array.isArray(actions) ? actions.length : 0, results: [] }
    }
    if (!actions || actions.length === 0) {
      setGroupBatchError('No actions to apply.')
      return { applied: 0, failed: 0, results: [] }
    }
    setGroupBatchError('')
    setGroupBatchStatus(`${statusPrefix}...`)
    try {
      const response = await batchGroups({ actions })
      const applied = Number(response?.applied || 0)
      const failed = Number(response?.failed || 0)
      const results = Array.isArray(response?.results) ? response.results : []
      const firstError = results.find((item) => item?.status !== 'applied' && item?.error)
      if (firstError) {
        setGroupBatchError(firstError.error)
      }
      setGroupBatchStatus(`${statusPrefix}: ${applied} applied, ${failed} failed.`)
      setGroupsStatus(`${statusPrefix}: ${applied} applied, ${failed} failed.`)
      loadGroups()
      loadDesired()
      return { applied, failed, results }
    } catch (err) {
      const msg = err.message || String(err)
      setGroupBatchError(msg)
      setGroupBatchStatus('')
      return { applied: 0, failed: actions.length, results: [] }
    }
  }

  function clearGroupSelection() {
    setSelectedGroupIds([])
    groupSelectionAnchorRef.current = -1
  }

  function handleGroupRowSelect(index, checked, shiftKey) {
    setSelectedGroupIds((prev) => {
      const next = new Set(prev)
      const anchor = groupSelectionAnchorRef.current
      const hasRange = shiftKey && anchor >= 0 && anchor < groups.length
      if (hasRange) {
        const start = Math.min(anchor, index)
        const end = Math.max(anchor, index)
        for (let i = start; i <= end; i += 1) {
          const id = groups[i]?.groupId
          if (!id) continue
          if (checked) next.add(id)
          else next.delete(id)
        }
      } else {
        const id = groups[index]?.groupId
        if (id) {
          if (checked) next.add(id)
          else next.delete(id)
        }
      }
      groupSelectionAnchorRef.current = index
      return groups.map((group) => group.groupId).filter((id) => next.has(id))
    })
  }

  function handleSelectAllGroups(checked) {
    if (checked) {
      setSelectedGroupIds(groups.map((group) => group.groupId))
      return
    }
    clearGroupSelection()
  }

  async function handleBulkDeleteSelectedGroups(forceCascade = false) {
    if (!canManageGroups) return
    if (forceCascade && !canDecommissionDevices) return
    if (selectedGroupIds.length === 0) return
    const cascadeDeviceIDs = listDevicesMatchingGroups(selectedGroupIds)
    let cascade = forceCascade
    const confirmed = window.confirm(
      `Delete ${selectedGroupIds.length} selected group(s)? This also clears desired state for those groups.`,
    )
    if (!confirmed) return

    if (cascadeDeviceIDs.length > 0 && canDecommissionDevices && !forceCascade) {
      cascade = window.confirm(
        `Also decommission ${cascadeDeviceIDs.length} matching device(s)?\n\nOK = delete groups + decommission devices\nCancel = delete groups only`,
      )
    } else if (!canDecommissionDevices) {
      cascade = false
    }

    const actions = selectedGroupIds.map((groupId) => ({ action: 'delete', groupId }))
    const res = await applyGroupBatchActions(actions, 'Bulk delete')
    if (res.failed === 0) {
      clearGroupSelection()
    }
    if (cascade && cascadeDeviceIDs.length > 0) {
      const appliedGroupIDs = new Set(
        (res.results || [])
          .filter((item) => item?.status === 'applied' && item?.groupId)
          .map((item) => item.groupId),
      )
      const matchedIDs = listDevicesMatchingGroups(Array.from(appliedGroupIDs))
      if (matchedIDs.length > 0) {
        const reason = promptDecommissionReason(
          `Enter decommission reason for ${matchedIDs.length} device(s) matched by deleted groups:`,
        )
        if (reason) {
          await decommissionDeviceIDs(matchedIDs, reason, 'Group cascade decommission')
        }
      }
    }
  }

  function selectedGroupsSnapshot() {
    const selectedSet = new Set(selectedGroupIds)
    return groups.filter((group) => selectedSet.has(group.groupId))
  }

  function listDevicesMatchingGroups(groupIDs) {
    if (!groupIDs || groupIDs.length === 0) return []
    const targetIDs = new Set(groupIDs)
    const matched = new Set()
    groups
      .filter((group) => targetIDs.has(group.groupId))
      .forEach((group) => {
        const selector = normalizeObject(group.selector)
        devices.forEach((device) => {
          if (selectorMatches(normalizeObject(device.labels), selector)) {
            matched.add(device.deviceId)
          }
        })
      })
    return Array.from(matched)
  }

  function openGroupMultiEdit() {
    if (!canManageGroups) return
    if (selectedGroupIds.length === 0) return
    setGroupMultiEditError('')
    setGroupMultiEditStatus('')
    setGroupMultiEditForm({
      region: '',
      role: '',
      site: '',
      custom: [{ key: '', value: '' }],
    })
    setGroupMultiEditOpen(true)
  }

  async function handleApplyGroupMultiEdit() {
    if (!canManageGroups) return
    const selected = selectedGroupsSnapshot()
    if (selected.length === 0) return
    setGroupMultiEditError('')
    setGroupMultiEditStatus('Applying group edits...')

    const customRows = (groupMultiEditForm.custom || [])
      .map((row) => ({ key: String(row.key || '').trim(), value: String(row.value || '') }))
      .filter((row) => row.key)
    const invalidCustom = customRows.some((row) => row.value === '')
    if (invalidCustom) {
      setGroupMultiEditError('Custom keys require values.')
      setGroupMultiEditStatus('')
      return
    }

    const hasAnyChange = Boolean(
      groupMultiEditForm.region ||
      groupMultiEditForm.role ||
      groupMultiEditForm.site ||
      customRows.length > 0,
    )
    if (!hasAnyChange) {
      setGroupMultiEditError('Set at least one value to apply.')
      setGroupMultiEditStatus('')
      return
    }

    const actions = selected.map((group) => {
      const selector = normalizeObject(group.selector)
      if (groupMultiEditForm.region) selector.region = groupMultiEditForm.region
      if (groupMultiEditForm.role) selector.role = groupMultiEditForm.role
      if (groupMultiEditForm.site) selector.site = groupMultiEditForm.site
      customRows.forEach((row) => {
        selector[row.key] = row.value
      })
      return {
        action: 'upsert',
        groupId: group.groupId,
        name: group.name || '',
        selector,
      }
    })
    const res = await applyGroupBatchActions(actions, 'Bulk group edit')
    setGroupMultiEditStatus(`Applied edits: ${res.applied} succeeded, ${res.failed} failed.`)
    if (res.failed === 0) {
      setGroupMultiEditOpen(false)
    }
  }

  function openGroupMultiDesired() {
    if (!canManageDesiredState) return
    if (selectedGroupIds.length === 0) return
    setGroupMultiDesiredError('')
    setGroupMultiDesiredStatus('')
    setGroupMultiDesiredForm({
      checkinIntervalSec: '',
      components: [newComponentRow()],
    })
    setGroupMultiDesiredOpen(true)
  }

  async function handleApplyGroupMultiDesired(e) {
    if (e) e.preventDefault()
    if (!canManageDesiredState) return
    const selected = selectedGroupsSnapshot()
    if (selected.length === 0) return
    setGroupMultiDesiredError('')
    setGroupMultiDesiredStatus('Applying desired state...')
    const { components, error } = buildComponentsPayload(groupMultiDesiredForm.components)
    if (error) {
      setGroupMultiDesiredError(error)
      setGroupMultiDesiredStatus('')
      return
    }
    const payload = {
      components,
      checkinIntervalSec: groupMultiDesiredForm.checkinIntervalSec
        ? Number(groupMultiDesiredForm.checkinIntervalSec)
        : undefined,
    }
    const results = await Promise.allSettled(
      selected.map((group) => setDesiredStateGroup(group.groupId, payload)),
    )
    const failed = results.filter((item) => item.status === 'rejected').length
    const succeeded = results.length - failed
    setGroupMultiDesiredStatus(`Set desired for ${succeeded} group(s), ${failed} failed.`)
    if (failed > 0) {
      const firstError = results.find((item) => item.status === 'rejected')
      if (firstError && firstError.status === 'rejected') {
        setGroupMultiDesiredError(firstError.reason?.message || String(firstError.reason))
      }
    } else {
      setGroupMultiDesiredOpen(false)
    }
    loadDesired()
  }

  async function handleGroupDeviceToggle(group, device, shouldAdd) {
    if (!canEditDeviceLabels) return
    const selector = normalizeObject(group.selector)
    const labels = normalizeObject(device.labels)
    const nextLabels = { ...labels }
    if (shouldAdd) {
      Object.entries(selector).forEach(([key, val]) => {
        nextLabels[key] = val
      })
    } else {
      Object.entries(selector).forEach(([key, val]) => {
        if (nextLabels[key] === val) {
          delete nextLabels[key]
        }
      })
    }
    setGroupsStatus(shouldAdd ? 'Adding device to group...' : 'Removing device from group...')
    try {
      await patchDevice(device.deviceId, { labels: nextLabels })
      setGroupsStatus('Device updated')
      loadDevices()
    } catch (err) {
      setGroupsError(err.message || String(err))
    }
  }

  async function handleClearDeviceOverride() {
    if (!canManageDesiredState) return
    if (!selectedDeviceId) return
    const ok = window.confirm('Clear device override and use group desired state?')
    if (!ok) return
    setDesiredStatus('Clearing device override...')
    try {
      await clearDesiredStateDevice(selectedDeviceId)
      setDesiredStatus('Device override cleared')
      loadDesired()
      setDeviceFormDirty(false)
    } catch (err) {
      setDesiredStatus(err.message || String(err))
    }
  }

  async function handleGroupDesired(e) {
    e.preventDefault()
    if (!canManageDesiredState) return
    setGroupsStatus('Setting group desired state...')
    const { components, error } = buildComponentsPayload(groupDesiredForm.components)
    if (error) {
      setGroupsStatus(error)
      return
    }
    const payload = {
      components,
      checkinIntervalSec: groupDesiredForm.checkinIntervalSec
        ? Number(groupDesiredForm.checkinIntervalSec)
        : undefined,
    }
    try {
      await setDesiredStateGroup(groupDesiredForm.groupId, payload)
      setGroupsStatus('Group desired state set')
      loadDesired()
      setGroupDesiredOpen(false)
    } catch (err) {
      setGroupsStatus(err.message || String(err))
    }
  }

  function handlePreviewGroupBulk() {
    if (!canManageGroups) return
    setGroupBulkError('')
    setGroupBulkStatus('')
    setGroupBulkRollbackCsv('')
    const plan = parseBulkGroupsCsv(groupBulkCsv, groups)
    setGroupBulkPreview(plan)
    if (plan.summary.totalRows === 0) {
      setGroupBulkError('No CSV rows found.')
    } else if (plan.summary.errorRows > 0) {
      setGroupBulkError(`Preview contains ${plan.summary.errorRows} row error(s). Fix and preview again.`)
    }
  }

  async function handleGroupBulkFileChange(event) {
    if (!canManageGroups) return
    const file = event.target.files?.[0]
    if (!file) return
    try {
      const text = await file.text()
      setGroupBulkCsv(text)
      setGroupBulkPreview(null)
      setGroupBulkError('')
      setGroupBulkStatus('')
      setGroupBulkRollbackCsv('')
    } catch (err) {
      setGroupBulkError(err.message || String(err))
    }
  }

  function downloadGroupBulkRollback() {
    if (!canManageGroups) return
    if (!groupBulkRollbackCsv) return
    const blob = new Blob([groupBulkRollbackCsv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'groups-rollback.csv'
    a.click()
    URL.revokeObjectURL(url)
  }

  function downloadGroupBulkTemplate() {
    if (!canManageGroups) return
    const template = [
      'action,groupId,name,region,role,site,selector_json,selector.customer',
      'upsert,,canary-west,west,edge,lab-1,,acme',
      'upsert,,prod-west,west,edge,site-a,"{""tier"":""prod""}",',
      'delete,00000000-0000-0000-0000-000000000000,,,,,,,',
    ].join('\n')
    const blob = new Blob([`${template}\n`], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'groups-bulk-template.csv'
    a.click()
    URL.revokeObjectURL(url)
  }

  async function handleApplyGroupBulk() {
    if (!canManageGroups) return
    const plan = groupBulkPreview || parseBulkGroupsCsv(groupBulkCsv, groups)
    setGroupBulkPreview(plan)
    if (plan.summary.errorRows > 0) {
      setGroupBulkError(`Cannot apply while ${plan.summary.errorRows} row error(s) exist.`)
      return
    }
    const applyRows = plan.rows.filter((row) => ['create', 'update', 'delete'].includes(row.outcome))
    if (applyRows.length === 0) {
      setGroupBulkStatus('Nothing to apply. All rows are no-change or skipped.')
      return
    }
    const ok = window.confirm(
      `Apply bulk group changes?\n` +
      `Create: ${plan.summary.createRows}\n` +
      `Update: ${plan.summary.updateRows}\n` +
      `Delete: ${plan.summary.deleteRows}`,
    )
    if (!ok) return

    setGroupBulkApplying(true)
    setGroupBulkError('')
    setGroupBulkStatus('Applying bulk group changes...')
    setGroupBulkRollbackCsv('')

    const existingByID = new Map(groups.map((group) => [group.groupId, group]))
    const rollbackRows = []
    const applyStateByRowID = new Map()
    const actions = applyRows.map((row) => (
      row.action === 'delete'
        ? { action: 'delete', groupId: row.groupId }
        : {
            action: 'upsert',
            groupId: row.groupId,
            name: row.name || '',
            selector: row.selector || {},
          }
    ))

    let success = 0
    let failed = 0
    try {
      const batch = await batchGroups({ actions })
      const results = Array.isArray(batch?.results) ? batch.results : []
      if (results.length === 0) {
        success = Number(batch?.applied || 0)
        failed = Number(batch?.failed || 0)
      } else {
        results.forEach((result) => {
          const idx = Number(result?.index)
          const row = Number.isInteger(idx) && idx >= 0 && idx < applyRows.length ? applyRows[idx] : null
          if (!row) return
          if (result?.status === 'applied') {
            applyStateByRowID.set(row.rowId, { applyStatus: 'applied', applyError: '' })
            success += 1
            const previous = existingByID.get(row.groupId)
            if (row.action === 'delete') {
              if (previous) {
                rollbackRows.push({
                  action: 'upsert',
                  groupId: previous.groupId,
                  name: previous.name || '',
                  selector: normalizeSelector(previous.selector),
                })
              }
            } else if (previous) {
              rollbackRows.push({
                action: 'upsert',
                groupId: previous.groupId,
                name: previous.name || '',
                selector: normalizeSelector(previous.selector),
              })
            } else {
              rollbackRows.push({
                action: 'delete',
                groupId: row.groupId,
                name: '',
                selector: {},
              })
            }
          } else {
            failed += 1
            applyStateByRowID.set(row.rowId, { applyStatus: 'failed', applyError: result?.error || 'batch action failed' })
          }
        })
      }
    } catch (err) {
      setGroupBulkApplying(false)
      setGroupBulkError(err.message || String(err))
      return
    }

    setGroupBulkPreview((current) => {
      if (!current) return current
      return {
        ...current,
        rows: current.rows.map((row) => {
          const state = applyStateByRowID.get(row.rowId)
          if (!state) return row
          return { ...row, applyStatus: state.applyStatus, applyError: state.applyError }
        }),
      }
    })
    if (rollbackRows.length > 0) {
      setGroupBulkRollbackCsv(buildBulkGroupsRollbackCsv(rollbackRows))
    }

    setGroupBulkStatus(`Bulk apply complete: ${success} succeeded, ${failed} failed.`)
    setGroupsStatus(`Bulk group apply: ${success} succeeded, ${failed} failed.`)
    setGroupBulkApplying(false)
    loadGroups()
    loadDesired()
  }

  async function handleClearGroupDesired() {
    if (!canManageDesiredState) return
    if (!selectedGroupId) return
    const ok = window.confirm('Clear desired state for this group?')
    if (!ok) return
    setGroupsStatus('Clearing group desired state...')
    try {
      await clearDesiredStateGroup(selectedGroupId)
      setGroupsStatus('Group desired state cleared')
      loadDesired()
      setGroupDesiredOpen(false)
    } catch (err) {
      setGroupsStatus(err.message || String(err))
    }
  }

  async function downloadLogs(deviceId) {
    try {
      const csv = await getDeviceLogs(deviceId)
      const blob = new Blob([csv], { type: 'text/csv' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `device-${deviceId}.csv`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      alert(err.message || String(err))
    }
  }

  async function loadLogs() {
    if (!logsDeviceId) return
    setLogLoading(true)
    setLogError('')
    try {
      const csv = await getDeviceLogs(logsDeviceId)
      const parsed = parseCSV(csv)
      const rows = parsed.rows.map((row) => {
        const timestamp = row[0]
        const ms = Date.parse(timestamp)
        return {
          timestamp,
          timestampMs: Number.isNaN(ms) ? null : ms,
          level: row[1],
          component: row[2],
          deviceId: row[3],
          message: row[4],
          fields: row[5],
        }
      })
      setLogRows(rows)
    } catch (err) {
      setLogError(err.message || String(err))
    } finally {
      setLogLoading(false)
    }
  }

  function buildEventParams() {
    return {
      type: eventType || undefined,
      deviceId: eventDeviceId || undefined,
      since: toIsoIfValid(eventFrom) || undefined,
      until: toIsoIfValid(eventTo) || undefined,
      limit: eventLimit ? Number(eventLimit) : undefined,
    }
  }

  async function loadEventsHistory() {
    setEventLoading(true)
    setEventQueryError('')
    try {
      const res = await listRuntimeEvents(buildEventParams())
      setEventRows(res.items || [])
    } catch (err) {
      setEventQueryError(err.message || String(err))
    } finally {
      setEventLoading(false)
    }
  }

  function buildAuditParams() {
    const params = {
      action: auditAction || undefined,
      actorType: auditActorType || undefined,
      actorId: auditActorId || undefined,
      targetType: auditTargetType || undefined,
      targetId: auditTargetId || undefined,
      status: auditStatus || undefined,
      since: toIsoIfValid(auditFrom) || undefined,
      until: toIsoIfValid(auditTo) || undefined,
      limit: auditLimit ? Number(auditLimit) : undefined,
    }
    return params
  }

  async function loadAudit() {
    if (!canViewAudit) {
      setAuditRows([])
      setAuditError('')
      setAuditLoading(false)
      return
    }
    setAuditLoading(true)
    setAuditError('')
    try {
      const res = await listAuditEvents(buildAuditParams())
      setAuditRows(res.items || [])
    } catch (err) {
      setAuditError(err.message || String(err))
    } finally {
      setAuditLoading(false)
    }
  }

  async function loadEnrollmentProfiles(options = {}) {
    const { silent = false } = options
    if (!canManagePendingEnrollments) {
      setEnrollmentProfiles([])
      setEnrollmentProfilesError('')
      if (!silent) setEnrollmentProfilesLoading(false)
      return
    }
    if (!silent) {
      setEnrollmentProfilesLoading(true)
    }
    setEnrollmentProfilesError('')
    try {
      const res = await listEnrollmentProfiles()
      setEnrollmentProfiles(res.items || [])
    } catch (err) {
      setEnrollmentProfilesError(err.message || String(err))
    } finally {
      if (!silent) {
        setEnrollmentProfilesLoading(false)
      }
    }
  }

  async function loadPendingEnrollments(options = {}) {
    const { silent = false } = options
    if (!canManagePendingEnrollments) {
      setPendingEnrollments([])
      setPendingEnrollmentsError('')
      if (!silent) setPendingEnrollmentsLoading(false)
      return
    }
    if (!silent) {
      setPendingEnrollmentsLoading(true)
    }
    setPendingEnrollmentsError('')
    try {
      const res = await listPendingEnrollments({
        status: pendingEnrollmentsFilter === 'all' ? undefined : pendingEnrollmentsFilter,
        limit: 200,
      })
      const items = res.items || []
      setPendingEnrollments(items)
      if (pendingEnrollmentsFilter === 'pending') {
        setPendingEnrollmentAlertCount(items.length)
      }
    } catch (err) {
      setPendingEnrollmentsError(err.message || String(err))
    } finally {
      if (!silent) {
        setPendingEnrollmentsLoading(false)
      }
    }
  }

  async function loadPendingEnrollmentAlertCount() {
    if (!canManagePendingEnrollments) {
      setPendingEnrollmentAlertCount(0)
      return
    }
    const res = await listPendingEnrollments({ status: 'pending', limit: 200 })
    setPendingEnrollmentAlertCount(Array.isArray(res?.items) ? res.items.length : 0)
  }

  async function handleCreateEnrollmentProfile() {
    if (!canManagePendingEnrollments) return
    const name = String(enrollmentProfileForm.name || '').trim()
    const challengeSecret = String(enrollmentProfileForm.challengeSecret || '').trim()
    const challengeHint = String(enrollmentProfileForm.challengeHint || '').trim()
    if (!name) {
      setEnrollmentProfilesError('Profile name is required.')
      return
    }
    const expiresInDays = Number(enrollmentProfileForm.expiresInDays || 0)
    const maxUses = Number(enrollmentProfileForm.maxUses || 0)
    const approvalDelaySec = Number(enrollmentProfileForm.approvalDelaySec || 0)
    if (!Number.isFinite(expiresInDays) || expiresInDays <= 0) {
      setEnrollmentProfilesError('Expiration days must be greater than 0.')
      return
    }
    if (!Number.isFinite(maxUses) || maxUses < 0) {
      setEnrollmentProfilesError('Max uses must be 0 or greater.')
      return
    }
    if (!Number.isFinite(approvalDelaySec) || approvalDelaySec < 0) {
      setEnrollmentProfilesError('Approval delay must be 0 or greater.')
      return
    }
    const labelsParsed = keyValueRowsToObject(enrollmentProfileForm.defaultLabelRows)
    if (labelsParsed.error) {
      setEnrollmentProfilesError(labelsParsed.error)
      return
    }
    const defaultLabels = labelsParsed.value
    setEnrollmentProfilesStatus('Creating enrollment profile...')
    setEnrollmentProfilesError('')
    try {
      const res = await createEnrollmentProfile({
        name,
        expiresInSec: Math.round(expiresInDays * 24 * 60 * 60),
        maxUses,
        requireApproval: true,
        allowUnsignedHardwareIdentity: Boolean(enrollmentProfileForm.allowUnsignedHardwareIdentity),
        challengeSecret,
        challengeHint,
        approvalDelaySec,
        ...(Object.keys(defaultLabels).length > 0 ? { defaultLabels } : {}),
      })
      setLatestEnrollmentProfileToken(res.bootstrapToken || '')
      setLatestEnrollmentProfileTokenName(res.name || name)
      setEnrollmentProfilesStatus(`Created profile ${res.name || name}`)
      setEnrollmentProfileForm((prev) => ({
        ...prev,
        challengeSecret: '',
        challengeHint: challengeSecret ? challengeHint : '',
        approvalDelaySec: String(approvalDelaySec),
        defaultLabelRows: objectToKeyValueRows(defaultLabels),
      }))
      setEnrollmentProfileCreateOpen(false)
      await loadEnrollmentProfiles({ silent: true })
    } catch (err) {
      setEnrollmentProfilesStatus('')
      setEnrollmentProfilesError(err.message || String(err))
    }
  }

  function handleOpenCreateEnrollmentProfile() {
    if (!canManagePendingEnrollments) return
    setEnrollmentProfilesError('')
    setEnrollmentProfilesStatus('')
    setEnrollmentProfileCreateOpen(true)
  }

  function handleStartEditEnrollmentProfile(profile) {
    if (!canManagePendingEnrollments) return
    setEditingEnrollmentProfileId(profile.profileId)
    setEnrollmentProfileEditForm({
      name: profile.name || '',
      maxUses: String(profile.maxUses ?? 0),
      challengeEnabled: Boolean(profile.challengeEnabled),
      challengeSecret: '',
      challengeHint: profile.challengeHint || '',
      approvalDelaySec: String(profile.approvalDelaySec ?? 0),
      defaultLabelRows: objectToKeyValueRows(profile.defaultLabels),
      allowUnsignedHardwareIdentity: Boolean(profile.allowUnsignedHardwareIdentity),
    })
    setEnrollmentProfilesError('')
    setEnrollmentProfilesStatus('')
  }

  function handleCancelEditEnrollmentProfile() {
    setEditingEnrollmentProfileId('')
    setEnrollmentProfileEditForm({
      name: '',
      maxUses: '0',
      challengeEnabled: false,
      challengeSecret: '',
      challengeHint: '',
      approvalDelaySec: '0',
      defaultLabelRows: [{ key: '', value: '' }],
      allowUnsignedHardwareIdentity: false,
    })
  }

  async function handleUpdateEnrollmentProfile() {
    if (!canManagePendingEnrollments || !editingEnrollmentProfileId) return
    const name = String(enrollmentProfileEditForm.name || '').trim()
    const challengeSecret = String(enrollmentProfileEditForm.challengeSecret || '').trim()
    const challengeHint = String(enrollmentProfileEditForm.challengeHint || '').trim()
    if (!name) {
      setEnrollmentProfilesError('Profile name is required.')
      return
    }
    const maxUses = Number(enrollmentProfileEditForm.maxUses || 0)
    const approvalDelaySec = Number(enrollmentProfileEditForm.approvalDelaySec || 0)
    if (!Number.isFinite(maxUses) || maxUses < 0) {
      setEnrollmentProfilesError('Max uses must be 0 or greater.')
      return
    }
    if (!Number.isFinite(approvalDelaySec) || approvalDelaySec < 0) {
      setEnrollmentProfilesError('Approval delay must be 0 or greater.')
      return
    }
    const labelsParsed = keyValueRowsToObject(enrollmentProfileEditForm.defaultLabelRows)
    if (labelsParsed.error) {
      setEnrollmentProfilesError(labelsParsed.error)
      return
    }
    const defaultLabels = labelsParsed.value
    const existing = enrollmentProfiles.find((profile) => profile.profileId === editingEnrollmentProfileId)
    const challengeEnabled = Boolean(enrollmentProfileEditForm.challengeEnabled)
    if (challengeEnabled && !existing?.challengeEnabled && !challengeSecret) {
      setEnrollmentProfilesError('Challenge secret is required when enabling challenge protection.')
      return
    }

    const payload = {
      name,
      maxUses,
      requireApproval: true,
      allowUnsignedHardwareIdentity: Boolean(enrollmentProfileEditForm.allowUnsignedHardwareIdentity),
      approvalDelaySec,
      ...(Object.keys(defaultLabels).length > 0 ? { defaultLabels } : { clearDefaultLabels: true }),
      ...(challengeEnabled
        ? {
            challengeHint,
            ...(challengeSecret ? { challengeSecret } : {}),
          }
        : { clearChallenge: true }),
    }

    setEnrollmentProfilesStatus('Updating enrollment profile...')
    setEnrollmentProfilesError('')
    try {
      await updateEnrollmentProfile(editingEnrollmentProfileId, payload)
      setEnrollmentProfilesStatus(`Updated ${name}`)
      await loadEnrollmentProfiles({ silent: true })
      handleCancelEditEnrollmentProfile()
    } catch (err) {
      setEnrollmentProfilesStatus('')
      setEnrollmentProfilesError(err.message || String(err))
    }
  }

  async function handleRotateEnrollmentProfile(profileId, name) {
    if (!canManagePendingEnrollments) return
    const confirmed = window.confirm(`Rotate bootstrap token for ${name || profileId}? Existing distributed tokens will stop working.`)
    if (!confirmed) return
    setEnrollmentProfilesStatus('Rotating bootstrap token...')
    setEnrollmentProfilesError('')
    try {
      const res = await rotateEnrollmentProfile(profileId)
      setLatestEnrollmentProfileToken(res.bootstrapToken || '')
      setLatestEnrollmentProfileTokenName(res.name || name || profileId)
      setEnrollmentProfilesStatus(`Rotated token for ${res.name || name || profileId}`)
      await loadEnrollmentProfiles({ silent: true })
    } catch (err) {
      setEnrollmentProfilesStatus('')
      setEnrollmentProfilesError(err.message || String(err))
    }
  }

  async function handleCopyEnrollmentProfileToken() {
    if (!latestEnrollmentProfileToken) return
    try {
      await copyText(latestEnrollmentProfileToken)
      setEnrollmentProfilesStatus(`Copied bootstrap token for ${latestEnrollmentProfileTokenName || 'profile'}`)
    } catch (err) {
      setEnrollmentProfilesError(err.message || String(err))
    }
  }

  function handleDownloadEnrollmentProfileToken() {
    if (!latestEnrollmentProfileToken) return
    const safeName = (latestEnrollmentProfileTokenName || 'enrollment-profile').replace(/[^a-z0-9._-]+/gi, '-')
    downloadTextFile(`${safeName}.bootstrap-token.txt`, latestEnrollmentProfileToken)
    setEnrollmentProfilesStatus(`Downloaded bootstrap token for ${latestEnrollmentProfileTokenName || 'profile'}`)
  }

  async function handleSetEnrollmentProfileDisabled(profileId, disabled) {
    if (!canManagePendingEnrollments) return
    setEnrollmentProfilesStatus(disabled ? 'Disabling enrollment profile...' : 'Enabling enrollment profile...')
    setEnrollmentProfilesError('')
    try {
      if (disabled) {
        await disableEnrollmentProfile(profileId)
      } else {
        await enableEnrollmentProfile(profileId)
      }
      setEnrollmentProfilesStatus(disabled ? `Disabled ${profileId}` : `Enabled ${profileId}`)
      await loadEnrollmentProfiles({ silent: true })
    } catch (err) {
      setEnrollmentProfilesStatus('')
      setEnrollmentProfilesError(err.message || String(err))
    }
  }

  async function handleApprovePendingEnrollment(requestId) {
    if (!canManagePendingEnrollments) return
    setPendingEnrollmentsStatus('Approving pending enrollment...')
    setPendingEnrollmentsError('')
    try {
      await approvePendingEnrollment(requestId)
      setPendingEnrollmentsStatus(`Approved ${requestId}`)
      await loadPendingEnrollments({ silent: true })
      await loadPendingEnrollmentAlertCount()
    } catch (err) {
      setPendingEnrollmentsStatus('')
      setPendingEnrollmentsError(err.message || String(err))
    }
  }

  async function handleResetPendingEnrollment(requestId) {
    if (!canManagePendingEnrollments) return
    setPendingEnrollmentsStatus('Resetting pending enrollment...')
    setPendingEnrollmentsError('')
    try {
      await resetPendingEnrollment(requestId)
      setPendingEnrollmentsStatus(`Reset ${requestId}`)
      await loadPendingEnrollments({ silent: true })
      await loadPendingEnrollmentAlertCount()
    } catch (err) {
      setPendingEnrollmentsStatus('')
      setPendingEnrollmentsError(err.message || String(err))
    }
  }

  async function handleDenyPendingEnrollment(requestId) {
    if (!canManagePendingEnrollments) return
    const reasonInput = window.prompt('Enter deny reason (optional):', '')
    if (reasonInput === null) return
    setPendingEnrollmentsStatus('Denying pending enrollment...')
    setPendingEnrollmentsError('')
    try {
      await denyPendingEnrollment(requestId, String(reasonInput || '').trim())
      setPendingEnrollmentsStatus(`Denied ${requestId}`)
      await loadPendingEnrollments({ silent: true })
      await loadPendingEnrollmentAlertCount()
    } catch (err) {
      setPendingEnrollmentsStatus('')
      setPendingEnrollmentsError(err.message || String(err))
    }
  }

  async function downloadAudit() {
    if (!canViewAudit) return
    try {
      const csv = await downloadAuditCSV(buildAuditParams())
      const blob = new Blob([csv], { type: 'text/csv' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'audit.csv'
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      alert(err.message || String(err))
    }
  }

  async function loadRotationStatus() {
    setRotationLoading(true)
    setRotationError('')
    try {
      const res = await getRotationStatus()
      setRotationStatus(res)
    } catch (err) {
      setRotationError(err.message || String(err))
    } finally {
      setRotationLoading(false)
    }
  }

  async function handleReloadRotation() {
    if (!canRotate) return
    setRotationMessage('Reloading CA files...')
    setRotationError('')
    try {
      const res = await reloadRotation()
      setRotationStatus(res)
      setRotationMessage('CA files reloaded')
    } catch (err) {
      setRotationMessage('')
      setRotationError(err.message || String(err))
    }
  }

  async function handleRotateRotation() {
    if (!canRotate) return
    const confirmed = window.confirm(
      'Rotate CA now? This will generate a new CA, rebuild the bundle, and trigger device re-enroll.',
    )
    if (!confirmed) return
    setRotationMessage('Rotating CA...')
    setRotationError('')
    try {
      const res = await rotateRotation()
      setRotationStatus(res)
      setRotationMessage('CA rotated')
    } catch (err) {
      setRotationMessage('')
      setRotationError(err.message || String(err))
    }
  }

  async function handleCleanupRotation() {
    if (!canRotate) return
    const confirmed = window.confirm(
      'Prune the old CA from the bundle? Devices still on the old CA will stop checking in once removed.',
    )
    if (!confirmed) return
    setRotationMessage('Cleaning up CA bundle...')
    setRotationError('')
    try {
      const res = await cleanupRotation()
      setRotationStatus(res)
      setRotationMessage('Old CA removed from bundle')
    } catch (err) {
      setRotationMessage('')
      setRotationError(err.message || String(err))
    }
  }

  async function loadAuditRetention() {
    if (!canManageAuditRetention) {
      setAuditRetentionState({ days: 90, updatedAt: '' })
      setAuditRetentionDays('90')
      setAuditRetentionStatus('')
      return
    }
    try {
      const res = await getAuditRetention()
      setAuditRetentionState(res)
      if (res?.days) {
        setAuditRetentionDays(String(res.days))
      }
    } catch (err) {
      setAuditRetentionStatus(err.message || String(err))
    }
  }

  async function loadEventRetention() {
    if (!canManageEventRetention) {
      setEventRetentionState({ days: 30, updatedAt: '' })
      setEventRetentionDays('30')
      setEventRetentionStatus('')
      return
    }
    try {
      const res = await getEventRetention()
      setEventRetentionState(res)
      if (res?.days) {
        setEventRetentionDays(String(res.days))
      }
    } catch (err) {
      setEventRetentionStatus(err.message || String(err))
    }
  }

  async function updateEventRetention() {
    if (!canManageEventRetention) return
    const value = Number(eventRetentionDays)
    if (!value || value <= 0) return
    setEventRetentionStatus('Updating retention...')
    try {
      const res = await setEventRetention(value)
      setEventRetentionState(res)
      setEventRetentionDays(String(res.days))
      setEventRetentionStatus('Retention updated')
      await loadEventsHistory()
    } catch (err) {
      setEventRetentionStatus(err.message || String(err))
    }
  }

  async function updateAuditRetention() {
    if (!canManageAuditRetention) return
    const value = Number(auditRetentionDays)
    if (!value || value <= 0) return
    setAuditRetentionStatus('Updating retention...')
    try {
      const res = await setAuditRetention(value)
      setAuditRetentionState(res)
      setAuditRetentionDays(String(res.days))
      setAuditRetentionStatus('Retention updated')
    } catch (err) {
      setAuditRetentionStatus(err.message || String(err))
    }
  }

  async function loadMaintenance() {
    setMaintenanceError('')
    try {
      const res = await getMaintenance()
      setMaintenanceState(res)
    } catch (err) {
      setMaintenanceError(err.message || String(err))
    }
  }

  async function loadUpgrade() {
    setUpgradeError('')
    try {
      const res = await getUpgradeStatus()
      setUpgrade(res)
    } catch (err) {
      setUpgradeError(err.message || String(err))
    }
  }

  async function loadUpgradeAvailable() {
    setUpgradeAvailableError('')
    try {
      const res = await getUpgradeAvailable()
      setUpgradeAvailable(res)
    } catch (err) {
      setUpgradeAvailableError(err.message || String(err))
    }
  }

  async function loadUpgradePreflight() {
    setUpgradePreflightStatus('Running preflight...')
    setUpgradePreflightError('')
    try {
      const res = await getUpgradePreflight()
      setUpgradePreflight(res)
      setUpgradePreflightStatus('')
    } catch (err) {
      setUpgradePreflightStatus('')
      setUpgradePreflightError(err.message || String(err))
    }
  }

  async function loadHealthSummary() {
    setHealthSummaryError('')
    try {
      const res = await getHealthSummary()
      setHealthSummary(res)
      const summary = res?.devices || {}
      const total = summary.total ?? devices.length
      const active = summary.active ?? devices.filter((d) => d.status === 'active').length
      const stale = summary.stale ?? devices.filter((d) => d.status === 'stale').length
      const offline = summary.offline ?? devices.filter((d) => d.status === 'offline').length
      const degraded = summary.degraded ?? devices.filter((d) => d.status === 'degraded').length
      const entry = {
        ts: Date.now(),
        total,
        active,
        stale,
        offline,
        degraded,
      }
      setHealthHistory((prev) => {
        const now = entry.ts
        const maxAge = rangeMsById['24h'] || 24 * 60 * 60 * 1000
        const trimmed = [...prev, entry].filter((point) => now - point.ts <= maxAge)
        if (trimmed.length > 2000) {
          return trimmed.slice(trimmed.length - 2000)
        }
        return trimmed
      })
    } catch (err) {
      setHealthSummaryError(err.message || String(err))
    }
  }

  async function loadMetrics() {
    if (!canViewMetrics) {
      setMetricsError('')
      setMetricsHistory([])
      setMetricsSnapshot({
        dbOpenConns: null,
        dbInUse: null,
        dbWaitCount: null,
        s3ObjectsTotal: null,
        s3BytesTotal: null,
        devicesTotal: null,
        pendingEnrollActive: null,
        pendingEnrollThrottleTotal: null,
        httpLatencyAvg: null,
        devicesStatus: {},
        updatedAt: '',
      })
      return
    }
    setMetricsError('')
    try {
      const text = await getMetricsText()
      const parsed = parsePrometheusMetrics(text)
      const devicesStatus = groupLabeledMetrics(parsed, 'hwops_devices_status_total', 'status')
      const checkinStatus = groupLabeledMetrics(parsed, 'hwops_checkin_total', 'status')
      const enrollStatus = groupLabeledMetrics(parsed, 'hwops_enroll_total', 'status')
      const enrollTokenStatus = groupLabeledMetrics(parsed, 'hwops_enrollment_token_total', 'status')
      const applyStatus = groupLabeledMetrics(parsed, 'hwops_apply_total', 'status')
      const preApplyStatus = groupLabeledMetrics(parsed, 'hwops_preapply_total', 'status')
      const uploadStatus = groupLabeledMetrics(parsed, 'hwops_artifact_upload_total', 'status')
      const presignStatus = groupLabeledMetrics(parsed, 'hwops_artifact_presign_total', 'status')
      const rateLimitByEndpoint = groupLabeledMetrics(parsed, 'hwops_rate_limit_total', 'endpoint')
      const pendingEnrollThrottleByReason = groupLabeledMetrics(parsed, 'hwops_pending_enroll_throttle_total', 'reason')
      const upgradeStatus = groupLabeledMetrics(parsed, 'hwops_upgrade_total', 'status')
      const backupStatus = groupLabeledMetrics(parsed, 'hwops_backup_total', 'operation', (label) => label || 'backup')
      const pendingByType = groupLabeledMetrics(parsed, 'hwops_pending_actions_total', 'type')
      const httpByStatus = groupLabeledMetrics(parsed, 'hwops_http_requests_total', 'status', (label) => {
        if (!label) return 'other'
        if (label.startsWith('2')) return '2xx'
        if (label.startsWith('3')) return '3xx'
        if (label.startsWith('4')) return '4xx'
        if (label.startsWith('5')) return '5xx'
        return 'other'
      })
      const httpLatencySum = sumLabeled(parsed, 'hwops_http_request_duration_seconds_sum')
      const httpLatencyCount = sumLabeled(parsed, 'hwops_http_request_duration_seconds_count')
      const httpLatencyAvg = httpLatencyCount > 0 ? (httpLatencySum / httpLatencyCount) * 1000 : null

      const snapshot = {
        ts: Date.now(),
        values: {
          dbOpenConns: parsed.values.hwops_db_open_conns ?? null,
          dbInUse: parsed.values.hwops_db_in_use ?? null,
          dbWaitCount: parsed.values.hwops_db_wait_count ?? null,
          s3ObjectsTotal: parsed.values.hwops_s3_objects_total ?? null,
          s3BytesTotal: parsed.values.hwops_s3_bytes_total ?? null,
          devicesTotal: parsed.values.hwops_devices_total ?? null,
          pendingEnrollActive: parsed.values.hwops_pending_enroll_active_total ?? null,
          pendingEnrollThrottleTotal: sumLabeled(parsed, 'hwops_pending_enroll_throttle_total') || 0,
          httpLatencyAvg,
        },
        labels: {
          devicesStatus,
          checkinStatus,
          enrollStatus,
          enrollTokenStatus,
          applyStatus,
          preApplyStatus,
          uploadStatus,
          presignStatus,
          rateLimitByEndpoint,
          pendingEnrollThrottleByReason,
          upgradeStatus,
          backupStatus,
          pendingByType,
          httpByStatus,
        },
      }

      setMetricsSnapshot({
        ...snapshot.values,
        devicesStatus,
        updatedAt: new Date().toISOString(),
      })
      setMetricsHistory((prev) => {
        const now = snapshot.ts
        const maxAge = rangeMsById['24h'] || 24 * 60 * 60 * 1000
        const trimmed = [...prev, snapshot].filter((point) => now - point.ts <= maxAge)
        if (trimmed.length > 2000) {
          return trimmed.slice(trimmed.length - 2000)
        }
        return trimmed
      })
    } catch (err) {
      setMetricsError(err.message || String(err))
    }
  }

  async function loadBackups() {
    if (!canViewBackups) {
      setBackups([])
      setSelectedBackupId('')
      setBackupError('')
      return
    }
    setBackupError('')
    try {
      const res = await listBackups()
      setBackups(res.items || [])
      if (!selectedBackupId && res.items && res.items.length > 0) {
        setSelectedBackupId(res.items[0].id)
      }
    } catch (err) {
      setBackupError(err.message || String(err))
    }
  }

  async function loadBackupStatus() {
    if (!canViewBackups) {
      setBackupStatus({ enabled: false, running: false, state: 'disabled' })
      setBackupError('')
      return
    }
    setBackupError('')
    try {
      const res = await getBackupStatus()
      setBackupStatus(res || { enabled: false, running: false, state: 'disabled' })
    } catch (err) {
      setBackupError(err.message || String(err))
    }
  }

  async function loadRestoreStatus() {
    if (!canViewBackups) {
      setRestoreStatus({ enabled: false, running: false, state: 'disabled' })
      setBackupError('')
      return
    }
    setBackupError('')
    try {
      const res = await getRestoreStatus()
      setRestoreStatus(res || { enabled: false, running: false, state: 'disabled' })
    } catch (err) {
      setBackupError(err.message || String(err))
    }
  }

  async function handleStartBackup() {
    if (!canManageBackups) return
    setBackupMessage('Starting backup...')
    setBackupError('')
    try {
      await startBackup()
      setBackupMessage('Backup started')
      loadBackupStatus()
      setTimeout(loadBackups, 1500)
    } catch (err) {
      setBackupMessage('')
      setBackupError(err.message || String(err))
    }
  }

  async function handleRestore() {
    if (!canManageBackups) return
    if (!selectedBackupId) {
      setBackupError('Select a backup to restore')
      return
    }
    const confirmed = window.confirm(
      `Restore backup ${selectedBackupId}? This will wipe the current database and object store.`,
    )
    if (!confirmed) return
    setBackupMessage('Starting restore...')
    setBackupError('')
    try {
      await startRestore(selectedBackupId)
      setBackupMessage('Restore started')
      loadRestoreStatus()
    } catch (err) {
      setBackupMessage('')
      setBackupError(err.message || String(err))
    }
  }

  async function toggleMaintenance() {
    if (!canManageMaintenance) return
    const nextEnabled = !maintenance.enabled
    let message = maintenance.message || ''
    if (nextEnabled) {
      const promptMsg = window.prompt('Maintenance message (optional):', message)
      if (promptMsg !== null) {
        message = promptMsg
      }
    } else {
      message = ''
    }
    setMaintenanceStatus(nextEnabled ? 'Enabling maintenance...' : 'Disabling maintenance...')
    try {
      const res = await setMaintenance({ enabled: nextEnabled, message })
      setMaintenanceState(res)
      setMaintenanceStatus(nextEnabled ? 'Maintenance enabled' : 'Maintenance disabled')
    } catch (err) {
      setMaintenanceStatus(err.message || String(err))
    }
  }

  async function startUpgrade() {
    if (!canApplyUpgrade || !upgrade.enabled) return
    if (!maintenance.enabled) {
      setUpgradeStatus('Enable maintenance before applying updates.')
      return
    }
    if (!upgradeAvailable.available) {
      setUpgradeStatus('No update bundle found in /stack/updates.')
      return
    }
    const proceed = window.confirm('Apply staged updates now?')
    if (!proceed) return
    setUpgradeStatus('Applying update...')
    try {
      const res = await applyUpgrade()
      setUpgrade(res)
      loadUpgradeAvailable()
      if (res.state === 'running') {
        setUpgradeStatus('Upgrade running...')
      } else {
        setUpgradeStatus(`Upgrade ${res.state || 'started'}`)
      }
    } catch (err) {
      setUpgradeStatus(err.message || String(err))
    }
  }

  const selectedDesired = desiredState.devices?.find((d) => d.deviceId === selectedDeviceId)
  const selectedGroupDesired = desiredState.groups?.find((g) => g.groupId === selectedGroupId)
  const lastAppliedArtifact = artifacts.find((a) => a.artifactId === deviceDetail?.current?.lastApplyArtifactId)
  const artifactByID = useMemo(() => {
    const map = {}
    artifacts.forEach((artifact) => {
      map[artifact.artifactId] = artifact
    })
    return map
  }, [artifacts])
  const artifactGroups = useMemo(() => {
    const map = new Map()
    artifacts.forEach((artifact) => {
      const name = artifact.name || 'unnamed'
      if (!map.has(name)) map.set(name, [])
      map.get(name).push(artifact)
    })
    return Array.from(map.entries())
      .map(([name, items]) => {
        const versions = [...items].sort((a, b) => a.version.localeCompare(b.version, undefined, { numeric: true }))
        return { name, versions }
      })
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [artifacts])
  const activeArtifactGroups = useMemo(() => {
    return artifactGroups
      .map((group) => ({
        ...group,
        versions: group.versions.filter((artifact) => normalizeArtifactStatus(artifact.status) !== 'deprecated'),
      }))
      .filter((group) => group.versions.length > 0)
  }, [artifactGroups])
  const artifactModalGroups = useMemo(() => {
    if (!artifactPickerTarget) return activeArtifactGroups
    const { scope, index } = artifactPickerTarget
    const row = scope === 'group'
      ? groupDesiredForm.components?.[index]
      : scope === 'groupBulk'
        ? groupMultiDesiredForm.components?.[index]
        : deviceForm.components?.[index]
    const type = normalizeArtifactType(row?.artifactType)
    if (!type) return activeArtifactGroups
    return filterArtifactGroupsByType(activeArtifactGroups, type)
  }, [activeArtifactGroups, artifactPickerTarget, deviceForm.components, groupDesiredForm.components, groupMultiDesiredForm.components])
  const selectedGroup = groups.find((g) => g.groupId === selectedGroupId)
  const selectedGroupSelector = selectedGroup ? normalizeObject(selectedGroup.selector) : {}
  const groupDeviceList = useMemo(() => {
    if (!selectedGroup) return []
    return devices.map((device) => ({
      device,
      inGroup: selectorMatches(normalizeObject(device.labels), selectedGroupSelector),
    }))
  }, [devices, selectedGroup, selectedGroupSelector])
  const groupCounts = useMemo(() => {
    const counts = {}
    groups.forEach((group) => {
      const selector = normalizeObject(group.selector)
      counts[group.groupId] = devices.filter((device) => selectorMatches(normalizeObject(device.labels), selector)).length
    })
    return counts
  }, [groups, devices])
  const selectedGroupSet = useMemo(() => new Set(selectedGroupIds), [selectedGroupIds])
  const allGroupsSelected = groups.length > 0 && selectedGroupIds.length === groups.length
  const someGroupsSelected = selectedGroupIds.length > 0 && !allGroupsSelected
  const selectedDeviceSet = useMemo(() => new Set(selectedDeviceIds), [selectedDeviceIds])
  const visibleArtifactRows = useMemo(
    () => artifactGroups.flatMap((group) => group.versions.map((artifact) => artifact.artifactId)),
    [artifactGroups],
  )
  const artifactRowIndexByID = useMemo(() => {
    const map = {}
    visibleArtifactRows.forEach((artifactId, index) => {
      map[artifactId] = index
    })
    return map
  }, [visibleArtifactRows])
  const selectedArtifactSet = useMemo(() => new Set(selectedArtifactIds), [selectedArtifactIds])
  const allArtifactsSelected =
    visibleArtifactRows.length > 0 && visibleArtifactRows.every((artifactId) => selectedArtifactSet.has(artifactId))
  const someArtifactsSelected = selectedArtifactIds.length > 0 && !allArtifactsSelected

  const groupSelectorById = useMemo(() => {
    const map = {}
    groups.forEach((group) => {
      map[group.groupId] = normalizeObject(group.selector)
    })
    return map
  }, [groups])

  const groupDesiredIds = useMemo(() => {
    return new Set((desiredState.groups || []).map((g) => g.groupId))
  }, [desiredState])

  const desiredDeviceById = useMemo(() => {
    const map = {}
    ;(desiredState.devices || []).forEach((d) => {
      map[d.deviceId] = d
    })
    return map
  }, [desiredState])

  const deviceSourceFor = (device) => {
    const entry = desiredDeviceById[device.deviceId]
    if (entry?.source === 'manual') return 'manual'
    for (const groupId of groupDesiredIds) {
      const selector = groupSelectorById[groupId]
      if (selectorMatches(normalizeObject(device.labels), selector)) {
        return 'group'
      }
    }
    if (entry?.source === 'agent') return 'agent'
    return 'agent'
  }
  const filteredLogs = useMemo(() => {
    const fromMs = logFrom ? Date.parse(logFrom) : null
    const toMs = logTo ? Date.parse(logTo) : null
    const withLevel = logRows.filter((row) => (logFilter === 'ALL' ? true : row.level === logFilter))
    const withTime = withLevel.filter((row) => {
      if (!fromMs && !toMs) return true
      if (!row.timestampMs) return false
      if (fromMs && row.timestampMs < fromMs) return false
      if (toMs && row.timestampMs > toMs) return false
      return true
    })
    const sorted = [...withTime].sort((a, b) => {
      const aMs = a.timestampMs ?? 0
      const bMs = b.timestampMs ?? 0
      return logSort === 'asc' ? aMs - bMs : bMs - aMs
    })
    return sorted
  }, [logRows, logFilter, logFrom, logTo, logSort])
  const liveEvents = eventsFeed.slice(0, 25)
  const healthRangeMs = rangeMsById[healthRange] || rangeMsById['1h']
  const metricsRangeMs = rangeMsById[metricsRange] || rangeMsById['1h']
  const healthSeries = useMemo(() => {
    const now = Date.now()
    const cutoff = now - healthRangeMs
    const points = healthHistory.filter((item) => item.ts >= cutoff)
    return [
      { id: 'total', label: 'Total', color: chartColors.total, points: points.map((p) => ({ ts: p.ts, value: p.total })) },
      { id: 'active', label: 'Active', color: chartColors.active, points: points.map((p) => ({ ts: p.ts, value: p.active })) },
      { id: 'degraded', label: 'Degraded', color: chartColors.degraded, points: points.map((p) => ({ ts: p.ts, value: p.degraded })) },
      { id: 'stale', label: 'Stale', color: chartColors.stale, points: points.map((p) => ({ ts: p.ts, value: p.stale })) },
      { id: 'offline', label: 'Offline', color: chartColors.offline, points: points.map((p) => ({ ts: p.ts, value: p.offline })) },
    ]
  }, [healthHistory, healthRangeMs])
  const dashboardSeries = useMemo(() => {
    return healthSeries.filter((entry) => entry.id === 'active')
  }, [healthSeries])

  const metricsLatest = metricsHistory[metricsHistory.length - 1]
  const metricsSeries = useMemo(() => {
    const history = metricsHistory
    if (history.length === 0) return {}
    const seriesFromKey = (key, label, color) => ({
      id: key,
      label,
      color,
      points: history.map((item) => ({ ts: item.ts, value: item.values?.[key] ?? 0 })),
    })
    const seriesFromLabels = (bucketKey, labels, colorMap = {}) =>
      labels.map((label) => ({
        id: `${bucketKey}-${label}`,
        label,
        color: colorMap[label] || chartColors[label] || '#8aa5ff',
        points: history.map((item) => ({ ts: item.ts, value: item.labels?.[bucketKey]?.[label] ?? 0 })),
      }))

    const rateLimitEntries = Object.entries(metricsLatest?.labels?.rateLimitByEndpoint || {})
    const rateLimitLabels = rateLimitEntries
      .sort((a, b) => b[1] - a[1])
      .slice(0, 3)
      .map(([label]) => label)
    const pendingEnrollThrottleEntries = Object.entries(metricsLatest?.labels?.pendingEnrollThrottleByReason || {})
    const pendingEnrollThrottleLabels = pendingEnrollThrottleEntries
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([label]) => label)
    const pendingLabels = Object.keys(metricsLatest?.labels?.pendingByType || {})
    const dynamicPalette = ['#8aa5ff', '#3bd487', '#f1c76f', '#f27272', '#9aa3ad']
    const rateLimitColors = rateLimitLabels.reduce((acc, label, idx) => {
      acc[label] = dynamicPalette[idx % dynamicPalette.length]
      return acc
    }, {})
    const pendingColors = pendingLabels.reduce((acc, label, idx) => {
      acc[label] = dynamicPalette[idx % dynamicPalette.length]
      return acc
    }, {})
    const pendingEnrollThrottleColors = pendingEnrollThrottleLabels.reduce((acc, label, idx) => {
      acc[label] = dynamicPalette[idx % dynamicPalette.length]
      return acc
    }, {})

    return {
      dbPool: [
        seriesFromKey('dbOpenConns', 'Open', '#8aa5ff'),
        seriesFromKey('dbInUse', 'In Use', '#3bd487'),
        seriesFromKey('dbWaitCount', 'Wait', '#f1c76f'),
      ],
      storageBytes: [seriesFromKey('s3BytesTotal', 'Bytes', '#8aa5ff')],
      storageObjects: [seriesFromKey('s3ObjectsTotal', 'Objects', '#3bd487')],
      pendingEnrollQueue: [seriesFromKey('pendingEnrollActive', 'Active Pending', '#f27272')],
      pendingEnrollThrottle: seriesFromLabels('pendingEnrollThrottleByReason', pendingEnrollThrottleLabels, pendingEnrollThrottleColors),
      devicesTotal: [seriesFromKey('devicesTotal', 'Devices', chartColors.total)],
      devicesStatus: seriesFromLabels('devicesStatus', ['active', 'degraded', 'stale', 'offline'], chartColors),
      checkins: seriesFromLabels('checkinStatus', ['success', 'error'], chartColors),
      enrollments: seriesFromLabels('enrollStatus', ['success', 'error'], chartColors),
      enrollTokens: seriesFromLabels('enrollTokenStatus', ['success', 'error'], chartColors),
      apply: seriesFromLabels('applyStatus', ['success', 'error'], chartColors),
      preApply: seriesFromLabels('preApplyStatus', ['success', 'error'], chartColors),
      uploads: seriesFromLabels('uploadStatus', ['success', 'error'], chartColors),
      presign: seriesFromLabels('presignStatus', ['success', 'error'], chartColors),
      upgrades: seriesFromLabels('upgradeStatus', ['success', 'error'], chartColors),
      backups: seriesFromLabels('backupStatus', ['backup', 'restore'], {
        backup: '#8aa5ff',
        restore: '#f1c76f',
      }),
      pending: seriesFromLabels('pendingByType', pendingLabels, pendingColors),
      rateLimit: seriesFromLabels('rateLimitByEndpoint', rateLimitLabels, rateLimitColors),
      httpStatus: seriesFromLabels('httpByStatus', ['2xx', '3xx', '4xx', '5xx', 'other'], {
        '2xx': '#3bd487',
        '3xx': '#8aa5ff',
        '4xx': '#f1c76f',
        '5xx': '#f27272',
        other: '#9aa3ad',
      }),
      httpLatency: [seriesFromKey('httpLatencyAvg', 'Avg ms', '#f1c76f')],
    }
  }, [metricsHistory, metricsLatest])

  const filteredDevices = useMemo(() => {
    if (deviceStatusFilter === 'all') return devices
    return devices.filter((d) => (d.status || '').toLowerCase() === deviceStatusFilter)
  }, [devices, deviceStatusFilter])
  const allDevicesSelected =
    filteredDevices.length > 0 && filteredDevices.every((device) => selectedDeviceSet.has(device.deviceId))
  const someDevicesSelected = selectedDeviceIds.length > 0 && !allDevicesSelected
  const authGatePending = !authStatus.loaded || (authStatus.enabled && authToken && !authUserLoaded)

  if (authGatePending) {
    return (
      <div className="login-screen">
        <div className="login-card">
          <div className="brand">HardwareOps</div>
          <div className="status">Checking authentication...</div>
        </div>
      </div>
    )
  }

  if (authStatus.loaded && authStatus.enabled && !authToken) {
    return (
      <div className="login-screen">
        <div className="login-card">
          <div className="brand">HardwareOps</div>
          <div className="auth-toggle">
            <button
              className={`tab ${authView === 'login' ? 'active' : ''}`}
              onClick={() => setAuthView('login')}
            >
              Sign in
            </button>
            <button
              className={`tab ${authView === 'register' ? 'active' : ''}`}
              onClick={() => setAuthView('register')}
            >
              Use voucher
            </button>
          </div>
          {authView === 'login' ? (
            <div className="form">
              {authStatus.oidcEnabled && (
                <a className="button" href={authStatus.oidcLoginURL} style={{ textAlign: 'center', marginBottom: '0.5rem' }}>
                  Sign in with SSO
                </a>
              )}
              <label>Email</label>
              <input
                value={loginForm.email}
                onChange={(e) => setLoginForm((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="admin@example.com"
              />
              <label>Password</label>
              <input
                type="password"
                value={loginForm.password}
                onChange={(e) => setLoginForm((prev) => ({ ...prev, password: e.target.value }))}
              />
              <button className="button" onClick={doLogin}>
                Sign in
              </button>
            </div>
          ) : (
            <div className="form">
              <label>Voucher token</label>
              <input
                value={registerForm.token}
                onChange={(e) => setRegisterForm((prev) => ({ ...prev, token: e.target.value }))}
                placeholder="paste token"
              />
              <label>Email</label>
              <input
                value={registerForm.email}
                onChange={(e) => setRegisterForm((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="user@example.com"
              />
              <label>Password</label>
              <input
                type="password"
                value={registerForm.password}
                onChange={(e) => setRegisterForm((prev) => ({ ...prev, password: e.target.value }))}
              />
              <label>Display Name</label>
              <input
                value={registerForm.displayName}
                onChange={(e) => setRegisterForm((prev) => ({ ...prev, displayName: e.target.value }))}
              />
              <button className="button" onClick={doRegister}>
                Create account
              </button>
            </div>
          )}
          {authError && <div className="error">{authError}</div>}
          {loginStatus && <div className="status">{loginStatus}</div>}
          {registerStatus && <div className="status">{registerStatus}</div>}
          <div className="status hint">
            Auth mode: {authStatus.mode}
          </div>
          {bootstrapState.enabled && (
            <div className="bootstrap-panel">
              <div className="detail-label">Bootstrap</div>
              {bootstrapState.tokenRequired && (
                <>
                  <label>{bootstrapState.tokenHeader || 'Bootstrap token'}</label>
                  <input
                    value={bootstrapToken}
                    onChange={(e) => setBootstrapToken(e.target.value)}
                    placeholder="paste bootstrap token"
                  />
                </>
              )}
              <button className="button ghost" onClick={doDownloadBootstrapCA}>
                Download CA Certificate
              </button>
              {bootstrapStatusMessage && <div className="status">{bootstrapStatusMessage}</div>}
            </div>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">HardwareOps</div>
        <nav className="nav">
          {visibleNav.map((item) => (
            <a
              key={item.id}
              href={`#${item.id}`}
              className={view === item.id ? 'active' : ''}
              onClick={() => setView(item.id)}
            >
              <span className={`nav-icon ${item.icon}`} />
              {item.label}
            </a>
          ))}
        </nav>
        <div className="sidebar-footer">
          <div className="env">
            API: {apiProxy ? `proxy → ${apiBaseUrl}` : apiBaseUrl}
            {simulateProd ? ' · prod-sim' : ''}
          </div>
          <button
            className="button ghost"
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          >
            {theme === 'dark' ? 'Light mode' : 'Dark mode'}
          </button>
        </div>
      </aside>

      <main className="content">
        {(maintenance.enabled || upgrade.running) && view !== 'settings' && (
          <div className="maintenance-overlay">
            <div className="maintenance-card">
              <div className="maintenance-title">
                {upgrade.running ? 'Upgrade in progress' : 'Maintenance mode'}
              </div>
              <div className="maintenance-text">
                {upgrade.running
                  ? 'An update is being applied. This page is temporarily read‑only.'
                  : (maintenance.message || 'Updates in progress. This page is temporarily read‑only.')}
              </div>
              <button className="button ghost" onClick={() => setView('settings')}>
                Open settings
              </button>
            </div>
          </div>
        )}
        {notifications.length > 0 && (
          <div className="notices">
            {notifications.map((n, idx) => (
              <div key={`${n.type}-${idx}`} className={`notice ${n.type}`}>
                <span>{n.text}</span>
                {n.actionLabel && typeof n.onAction === 'function' && (
                  <button className="notice-action" type="button" onClick={n.onAction}>
                    {n.actionLabel}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
        {view === 'dashboard' && (
          <>
            <section id="dashboard" className="card">
              <div className="section-header">
                <h2>Dashboard</h2>
                <div className="inline-row">
                  <div className="range-toggle">
                    {rangeOptions.map((opt) => (
                      <button
                        key={opt.id}
                        className={`chip ${healthRange === opt.id ? 'active' : ''}`}
                        onClick={() => setHealthRange(opt.id)}
                        type="button"
                      >
                        {opt.label}
                      </button>
                    ))}
                  </div>
                  <button className="button ghost" onClick={loadHealthSummary}>Refresh</button>
                </div>
              </div>
              {healthSummaryError && <div className="error">{healthSummaryError}</div>}
              <div className="grid">
                <div className="metric">
                  <div className="metric-label">Devices</div>
                  <div className="metric-value">{dashboard.total}</div>
                </div>
                <div className="metric">
                  <div className="metric-label">Active</div>
                  <div className="metric-value">{dashboard.active}</div>
                </div>
                <div className="metric">
                  <div className="metric-label">Degraded</div>
                  <div className="metric-value">{dashboard.degraded}</div>
                </div>
                <div className="metric">
                  <div className="metric-label">Stale</div>
                  <div className="metric-value">{dashboard.stale}</div>
                </div>
                <div className="metric">
                  <div className="metric-label">Offline</div>
                  <div className="metric-value">{dashboard.offline}</div>
                </div>
                <div className="metric">
                  <div className="metric-label">Last Check-in</div>
                  <div className="metric-value small">{dashboard.lastSeenLabel}</div>
                </div>
              </div>
              <div className="chart-card">
                <div className="chart-header">
                  <h3>Active Devices Over Time</h3>
                  <ChartLegend series={dashboardSeries} />
                </div>
                <TimeSeriesChart series={dashboardSeries} rangeMs={healthRangeMs} integerOnly />
              </div>

            </section>

            <section id="devices" className="card">
              <div className="section-header">
                <h2>Devices</h2>
                <div className="inline-row">
                  <select
                    value={deviceStatusFilter}
                    onChange={(e) => setDeviceStatusFilter(e.target.value)}
                  >
                    <option value="all">All</option>
                    <option value="active">Active</option>
                    <option value="degraded">Degraded</option>
                    <option value="stale">Stale</option>
                    <option value="offline">Offline</option>
                  </select>
                  <button onClick={loadDevices} className="button">Refresh</button>
                </div>
              </div>
              {devicesError && <div className="error">{devicesError}</div>}
              {selectedDeviceIds.length > 1 && (
                <div className="group-selection-toolbar">
                  <div className="group-selection-count">{selectedDeviceIds.length} selected</div>
                  <div className="inline-row">
                    {canDecommissionDevices && (
                      <button
                        className="button"
                        onClick={handleBulkDecommissionSelectedDevices}
                        disabled={selectedDeviceIds.length === 0}
                      >
                        Decommission Selected
                      </button>
                    )}
                    <button className="button ghost" onClick={clearDeviceSelection} disabled={selectedDeviceIds.length === 0}>
                      Clear
                    </button>
                  </div>
                </div>
              )}
              {devicesLoading ? (
                <div className="placeholder">Loading devices...</div>
              ) : (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>
                          <input
                            type="checkbox"
                            checked={allDevicesSelected}
                            ref={(el) => {
                              if (el) {
                                el.indeterminate = someDevicesSelected
                              }
                            }}
                            onChange={(e) => handleSelectAllDevices(e.target.checked)}
                          />
                        </th>
                        <th>Device ID</th>
                        <th>Status</th>
                        <th>Last Seen</th>
                        <th>Labels</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredDevices.map((d, index) => (
                        <tr
                          key={d.deviceId}
                          className={selectedDeviceId === d.deviceId ? 'selected' : ''}
                          onClick={() => {
                            setSelectedDeviceId(d.deviceId)
                            setDeviceDrawerOpen(true)
                            setDeviceFormDirty(false)
                          }}
                        >
                          <td>
                            <input
                              type="checkbox"
                              checked={selectedDeviceSet.has(d.deviceId)}
                              onClick={(e) => e.stopPropagation()}
                              onChange={(e) => handleDeviceRowSelect(index, e.target.checked, Boolean(e.nativeEvent?.shiftKey))}
                            />
                          </td>
                          <td>{d.deviceId}</td>
                          <td>
                            <span className={`pill status ${(d.status || 'unknown').toLowerCase()}`}>
                              {d.status || 'unknown'}
                            </span>
                            {deviceSourceFor(d) === 'manual' && (
                              <span className="pill source manual">manual</span>
                            )}
                            {deviceSourceFor(d) === 'group' && (
                              <span className="pill source group">group</span>
                            )}
                            {deviceSourceFor(d) === 'agent' && (
                              <span className="pill source agent">agent</span>
                            )}
                          </td>
                          <td>{d.lastSeen || '—'}</td>
                          <td><code>{d.labels ? JSON.stringify(d.labels) : '—'}</code></td>
                        </tr>
                      ))}
                      {filteredDevices.length === 0 && (
                        <tr>
                          <td colSpan={5}>No devices match this filter.</td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            <section id="groups" className="card">
              <div className="section-header">
                <h2>Groups</h2>
                <div className="inline-row">
                  {canManageGroups && (
                    <button
                      onClick={() => {
                        setGroupForm({ groupId: '', name: '', region: '', role: '', site: '', custom: [] })
                        setGroupModalOpen(true)
                      }}
                      className="button"
                    >
                      Add Group
                    </button>
                  )}
                  {canManageGroups && (
                    <button
                      onClick={() => {
                        setGroupBulkOpen(true)
                        setGroupBulkError('')
                        setGroupBulkStatus('')
                      }}
                      className="button ghost"
                    >
                      Advanced CSV
                    </button>
                  )}
                  <button onClick={loadGroups} className="button ghost">Refresh</button>
                </div>
              </div>
              {groupsError && <div className="error">{groupsError}</div>}
              {groupBatchError && <div className="error">{groupBatchError}</div>}

              {selectedGroupIds.length > 1 && (
                <div className="group-selection-toolbar">
                  <div className="group-selection-count">{selectedGroupIds.length} selected</div>
                  <div className="inline-row">
                    {canManageGroups && (
                      <button className="button ghost" onClick={openGroupMultiEdit} disabled={selectedGroupIds.length === 0}>
                        Edit Selected
                      </button>
                    )}
                    {canManageDesiredState && (
                      <button className="button ghost" onClick={openGroupMultiDesired} disabled={selectedGroupIds.length === 0}>
                        Set Desired Selected
                      </button>
                    )}
                    {canManageGroups && (
                      <button className="button" onClick={() => handleBulkDeleteSelectedGroups(false)} disabled={selectedGroupIds.length === 0}>
                        Delete Selected
                      </button>
                    )}
                    {canManageGroups && canDecommissionDevices && (
                      <button className="button ghost" onClick={() => handleBulkDeleteSelectedGroups(true)} disabled={selectedGroupIds.length === 0}>
                        Delete + Devices
                      </button>
                    )}
                    <button className="button ghost" onClick={clearGroupSelection} disabled={selectedGroupIds.length === 0}>
                      Clear
                    </button>
                  </div>
                  {groupBatchStatus && <div className="status">{groupBatchStatus}</div>}
                </div>
              )}

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>
                        <input
                          type="checkbox"
                          checked={allGroupsSelected}
                          ref={(el) => {
                            if (el) {
                              el.indeterminate = someGroupsSelected
                            }
                          }}
                          onChange={(e) => handleSelectAllGroups(e.target.checked)}
                        />
                      </th>
                      <th>Name</th>
                      <th>Selector</th>
                      <th>Devices</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {groups.map((group, index) => (
                      <tr key={group.groupId} className={selectedGroupId === group.groupId ? 'selected' : ''}>
                        <td>
                          <input
                            type="checkbox"
                            checked={selectedGroupSet.has(group.groupId)}
                            onChange={(e) => handleGroupRowSelect(index, e.target.checked, Boolean(e.nativeEvent?.shiftKey))}
                          />
                        </td>
                        <td>{group.name || group.groupId}</td>
                        <td><code>{formatSelector(group.selector || {})}</code></td>
                        <td>{groupCounts[group.groupId] ?? 0}</td>
                        <td>
                          {canManageGroups && (
                            <button
                              className="button ghost"
                              onClick={() => {
                                const parsed = selectorToForm(normalizeObject(group.selector))
                                setGroupForm({
                                  groupId: group.groupId,
                                  name: group.name || '',
                                  region: parsed.region,
                                  role: parsed.role,
                                  site: parsed.site,
                                  custom: parsed.custom,
                                })
                                setGroupModalOpen(true)
                              }}
                            >
                              Edit
                            </button>
                          )}
                          <button
                            className="button ghost"
                            onClick={() => setSelectedGroupId(group.groupId)}
                          >
                            {canEditDeviceLabels ? 'Manage Devices' : 'View Devices'}
                          </button>
                          {canManageDesiredState && (
                            <button
                              className="button ghost"
                              onClick={() => {
                                setSelectedGroupId(group.groupId)
                                setGroupDesiredOpen(true)
                              }}
                            >
                              Set Desired
                            </button>
                          )}
                          {canManageGroups && (
                            <button
                              className="button ghost"
                              onClick={() => handleDeleteGroup(group.groupId, false)}
                            >
                              Delete
                            </button>
                          )}
                          {canManageGroups && canDecommissionDevices && (
                            <button
                              className="button ghost"
                              onClick={() => handleDeleteGroup(group.groupId, true)}
                            >
                              Delete + Devices
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                    {groups.length === 0 && (
                      <tr>
                        <td colSpan={5}>No groups yet.</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>

              {selectedGroup && (
                <div className="group-devices">
                  <div className="section-header">
                    <h3>Group Devices · {selectedGroup.name || selectedGroup.groupId}</h3>
                    <button className="button ghost" onClick={() => setSelectedGroupId('')}>
                      Close
                    </button>
                  </div>
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>Device</th>
                          <th>Status</th>
                          <th>Labels</th>
                          <th>Membership</th>
                          <th>Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {groupDeviceList.map((item) => (
                          <tr key={item.device.deviceId}>
                            <td>{item.device.deviceId}</td>
                            <td>{item.device.status}</td>
                            <td><code>{JSON.stringify(item.device.labels || {})}</code></td>
                            <td>{item.inGroup ? 'in group' : '—'}</td>
                            <td>
                              {canEditDeviceLabels ? (
                                <button
                                  className="button ghost"
                                  onClick={() => handleGroupDeviceToggle(selectedGroup, item.device, !item.inGroup)}
                                  disabled={Object.keys(selectedGroupSelector).length === 0}
                                >
                                  {item.inGroup ? 'Remove' : 'Add'}
                                </button>
                              ) : '—'}
                            </td>
                          </tr>
                        ))}
                        {groupDeviceList.length === 0 && (
                          <tr>
                            <td colSpan={5}>No devices available.</td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                  {Object.keys(selectedGroupSelector).length === 0 && (
                    <div className="hint">
                      {canEditDeviceLabels
                        ? 'Empty selector matches all devices. Add keys to enable membership control.'
                        : 'Empty selector matches all devices.'}
                    </div>
                  )}
                </div>
              )}
            </section>

            <section id="artifacts" className="card">
              <div className="section-header">
                <h2>Artifacts</h2>
                <div className="inline-row">
                  <button onClick={() => setArtifactUploadOpen(true)} className="button" disabled={!canManageArtifacts}>
                    Upload
                  </button>
                  <button
                    onClick={handlePruneArtifacts}
                    className="button ghost"
                    disabled={!canManageArtifactLifecycle}
                  >
                    Prune now
                  </button>
                  <button onClick={loadArtifacts} className="button ghost">Refresh</button>
                </div>
              </div>
              {artifactsError && <div className="error">{artifactsError}</div>}
              {selectedArtifactIds.length > 1 && (
                <div className="group-selection-toolbar">
                  <div className="group-selection-count">{selectedArtifactIds.length} selected</div>
                  <div className="inline-row">
                    <button
                      className="button"
                      onClick={handleBulkDeprecateSelectedArtifacts}
                      disabled={!canManageArtifacts || selectedArtifactIds.length === 0}
                    >
                      Deprecate Selected
                    </button>
                    <button
                      className="button"
                      onClick={handleBulkDeleteSelectedArtifacts}
                      disabled={!canManageArtifacts || selectedArtifactIds.length === 0}
                    >
                      Delete Selected
                    </button>
                    <button className="button ghost" onClick={clearArtifactSelection} disabled={selectedArtifactIds.length === 0}>
                      Clear
                    </button>
                  </div>
                </div>
              )}

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>
                        <input
                          type="checkbox"
                          checked={allArtifactsSelected}
                          ref={(el) => {
                            if (el) {
                              el.indeterminate = someArtifactsSelected
                            }
                          }}
                          onChange={(e) => handleSelectAllArtifacts(e.target.checked)}
                        />
                      </th>
                      <th>Name</th>
                      <th>Artifact ID</th>
                      <th>Version</th>
                      <th>Type</th>
                      <th>Lifecycle</th>
                      <th>Signed</th>
                      <th>Refs</th>
                      <th>Delete After</th>
                      <th>Created</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {artifactGroups.map((group) => (
                      group.versions.map((a, idx) => {
                        const lifecycle = normalizeArtifactStatus(a.status)
                        const refs = Number(a.referenceCount || 0)
                        const canDeleteArtifactVersion =
                          canManageArtifacts && lifecycle === 'deprecated' && refs <= 0
                        return (
                          <tr key={a.artifactId} className={idx === 0 ? 'artifact-group-start' : ''}>
                            <td>
                              <input
                                type="checkbox"
                                checked={selectedArtifactSet.has(a.artifactId)}
                                onChange={(e) =>
                                  handleArtifactRowSelect(
                                    artifactRowIndexByID[a.artifactId] ?? 0,
                                    e.target.checked,
                                    Boolean(e.nativeEvent?.shiftKey),
                                  )}
                              />
                            </td>
                            <td>{idx === 0 ? group.name : ''}</td>
                            <td className="mono">{a.artifactId}</td>
                            <td>{a.version}</td>
                            <td>{a.type || 'app_bundle'}</td>
                            <td>
                              <span className={`pill artifact-status ${lifecycle}`}>
                                {lifecycle}
                              </span>
                            </td>
                            <td>
                              <span className={`pill ${a.signature ? 'signed' : 'unsigned'}`}>
                                {a.signature ? 'signed' : 'unsigned'}
                              </span>
                            </td>
                            <td>{refs}</td>
                            <td>{lifecycle === 'deprecated' ? formatTime(a.deleteAfter) : '—'}</td>
                            <td>{formatTime(a.createdAt)}</td>
                            <td className="artifact-actions">
                              {lifecycle === 'deprecated' ? (
                                <button
                                  className="button artifact-action-main"
                                  onClick={() => handleRestoreArtifact(a.artifactId)}
                                  disabled={!canManageArtifacts}
                                >
                                  Restore
                                </button>
                              ) : (
                                <button
                                  className="button artifact-action-main"
                                  onClick={() => handleDeprecateArtifact(a.artifactId)}
                                  disabled={!canManageArtifacts}
                                >
                                  Deprecate
                                </button>
                              )}
                              <button
                                className={lifecycle === 'deprecated' ? 'button' : 'button ghost muted'}
                                onClick={() => handleDeleteArtifact(a.artifactId)}
                                disabled={!canDeleteArtifactVersion}
                              >
                                Delete
                              </button>
                            </td>
                          </tr>
                        )
                      })
                    ))}
                    {artifactGroups.length === 0 && (
                      <tr>
                        <td colSpan={11}>No artifacts uploaded yet.</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          </>
        )}

        {view === 'metrics' && canViewMetrics && (
          <section id="metrics" className="card metrics-page">
            <div className="section-header">
              <h2>System Metrics</h2>
              <div className="inline-row">
                <div className="range-toggle">
                  {rangeOptions.map((opt) => (
                    <button
                      key={opt.id}
                      className={`chip ${metricsRange === opt.id ? 'active' : ''}`}
                      onClick={() => setMetricsRange(opt.id)}
                      type="button"
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
                <button className="button ghost" onClick={loadMetrics}>Refresh</button>
              </div>
            </div>
            {metricsError && <div className="error">{metricsError}</div>}
            <div className="grid metrics-summary">
              <div className="metric">
                <div className="metric-label">DB Open</div>
                <div className="metric-value">{formatNumber(metricsSnapshot.dbOpenConns)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">DB In Use</div>
                <div className="metric-value">{formatNumber(metricsSnapshot.dbInUse)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">DB Wait</div>
                <div className="metric-value">{formatNumber(metricsSnapshot.dbWaitCount)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">Objects</div>
                <div className="metric-value">{formatNumber(metricsSnapshot.s3ObjectsTotal)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">Storage</div>
                <div className="metric-value">{formatBytes(metricsSnapshot.s3BytesTotal)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">Pending Queue</div>
                <div className="metric-value">{formatNumber(metricsSnapshot.pendingEnrollActive)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">Throttle Hits</div>
                <div className="metric-value">{formatNumber(metricsSnapshot.pendingEnrollThrottleTotal)}</div>
              </div>
              <div className="metric">
                <div className="metric-label">Updated</div>
                <div className="metric-value small">{formatTime(metricsSnapshot.updatedAt)}</div>
              </div>
            </div>

            {metricsHistory.length === 0 && !metricsError && (
              <div className="placeholder">Metrics will appear after the first sample.</div>
            )}

            <div className="metrics-grid">
              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>DB Pool</h3>
                </div>
                <ChartLegend series={metricsSeries.dbPool} />
                <TimeSeriesChart series={metricsSeries.dbPool} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Storage (Bytes)</h3>
                </div>
                <ChartLegend series={metricsSeries.storageBytes} />
                <TimeSeriesChart series={metricsSeries.storageBytes} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Storage (Objects)</h3>
                </div>
                <ChartLegend series={metricsSeries.storageObjects} />
                <TimeSeriesChart series={metricsSeries.storageObjects} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Pending Enrollment Queue</h3>
                </div>
                <ChartLegend series={metricsSeries.pendingEnrollQueue} />
                <TimeSeriesChart series={metricsSeries.pendingEnrollQueue} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Pending Enrollment Throttles</h3>
                </div>
                <ChartLegend series={metricsSeries.pendingEnrollThrottle} />
                <TimeSeriesChart series={metricsSeries.pendingEnrollThrottle} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Device Status</h3>
                </div>
                <ChartLegend series={metricsSeries.devicesStatus} />
                <TimeSeriesChart series={metricsSeries.devicesStatus} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Check-ins</h3>
                </div>
                <ChartLegend series={metricsSeries.checkins} />
                <TimeSeriesChart series={metricsSeries.checkins} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Enrollments</h3>
                </div>
                <ChartLegend series={metricsSeries.enrollments} />
                <TimeSeriesChart series={metricsSeries.enrollments} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Enrollment Tokens</h3>
                </div>
                <ChartLegend series={metricsSeries.enrollTokens} />
                <TimeSeriesChart series={metricsSeries.enrollTokens} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Apply Results</h3>
                </div>
                <ChartLegend series={metricsSeries.apply} />
                <TimeSeriesChart series={metricsSeries.apply} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Pre-apply Results</h3>
                </div>
                <ChartLegend series={metricsSeries.preApply} />
                <TimeSeriesChart series={metricsSeries.preApply} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Artifact Uploads</h3>
                </div>
                <ChartLegend series={metricsSeries.uploads} />
                <TimeSeriesChart series={metricsSeries.uploads} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Artifact Presign</h3>
                </div>
                <ChartLegend series={metricsSeries.presign} />
                <TimeSeriesChart series={metricsSeries.presign} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Rate Limit Hits</h3>
                </div>
                <ChartLegend series={metricsSeries.rateLimit} />
                <TimeSeriesChart series={metricsSeries.rateLimit} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Pending Actions</h3>
                </div>
                <ChartLegend series={metricsSeries.pending} />
                <TimeSeriesChart series={metricsSeries.pending} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Upgrades</h3>
                </div>
                <ChartLegend series={metricsSeries.upgrades} />
                <TimeSeriesChart series={metricsSeries.upgrades} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>Backups</h3>
                </div>
                <ChartLegend series={metricsSeries.backups} />
                <TimeSeriesChart series={metricsSeries.backups} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>HTTP Requests</h3>
                </div>
                <ChartLegend series={metricsSeries.httpStatus} />
                <TimeSeriesChart series={metricsSeries.httpStatus} rangeMs={metricsRangeMs} integerOnly />
              </div>

              <div className="metrics-card">
                <div className="metrics-card-header">
                  <h3>HTTP Latency (avg ms)</h3>
                </div>
                <ChartLegend series={metricsSeries.httpLatency} />
                <TimeSeriesChart series={metricsSeries.httpLatency} rangeMs={metricsRangeMs} />
              </div>
            </div>
          </section>
        )}

        {view === 'logs' && (
          <section id="logs" className="card logs-card">
            <div className="section-header logs-header">
              <div className="tab-bar">
                {visibleLogsTabs.map((tab) => (
                  <button
                    key={tab.id}
                    className={`tab ${logsTab === tab.id ? 'active' : ''}`}
                    onClick={() => setLogsTab(tab.id)}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>
              <div className="logs-actions">
                {logsTab === 'events' && (
                  <>
                    <span className={`pill ${eventsStatus}`}>{eventsStatus}</span>
                    <button className="button" onClick={loadEventsHistory}>
                      Fetch History
                    </button>
                    <button className="button ghost" onClick={() => setEventsFeed([])}>
                      Clear
                    </button>
                  </>
                )}
                {logsTab === 'device' && (
                  <>
                    <select value={logFilter} onChange={(e) => setLogFilter(e.target.value)}>
                      <option value="ALL">All</option>
                      <option value="DEBUG">DEBUG</option>
                      <option value="INFO">INFO</option>
                      <option value="WARN">WARN</option>
                      <option value="ERROR">ERROR</option>
                    </select>
                    <select value={logSort} onChange={(e) => setLogSort(e.target.value)}>
                      <option value="desc">Newest</option>
                      <option value="asc">Oldest</option>
                    </select>
                    <button className="button" onClick={loadLogs}>
                      Fetch Logs
                    </button>
                  </>
                )}
                {logsTab === 'audit' && canViewAudit && (
                  <>
                    <button className="button" onClick={loadAudit}>
                      Fetch Audit
                    </button>
                    <button className="button ghost" onClick={downloadAudit}>
                      Download CSV
                    </button>
                  </>
                )}
              </div>
            </div>

            {logsTab === 'events' && (
              <div className="events">
                {eventsError && <div className="error">{eventsError}</div>}
                {eventQueryError && <div className="error">{eventQueryError}</div>}
                <div className="events-feed scroll">
                  {liveEvents.length === 0 ? (
                    <div className="placeholder">No events yet.</div>
                  ) : (
                    liveEvents.map((evt, idx) => (
                      <div key={`${evt.at}-${idx}`} className="event-item">
                        <div className="event-head">
                          <span className="event-type">{evt.type}</span>
                          <span className="event-time">{evt.at}</span>
                        </div>
                        <div className="event-meta">
                          Device: {evt.deviceId || '—'}
                        </div>
                      </div>
                    ))
                  )}
                </div>
                <div className="form inline">
                  <label>Type</label>
                  <input
                    value={eventType}
                    onChange={(e) => setEventType(e.target.value)}
                    placeholder="device.checkin"
                  />
                  <label>Device ID</label>
                  <input
                    value={eventDeviceId}
                    onChange={(e) => setEventDeviceId(e.target.value)}
                    placeholder="device uuid"
                  />
                  <label>From</label>
                  <input
                    type="datetime-local"
                    value={eventFrom}
                    onChange={(e) => setEventFrom(e.target.value)}
                  />
                  <label>To</label>
                  <input
                    type="datetime-local"
                    value={eventTo}
                    onChange={(e) => setEventTo(e.target.value)}
                  />
                  <label>Limit</label>
                  <input
                    type="number"
                    min="1"
                    max="5000"
                    value={eventLimit}
                    onChange={(e) => setEventLimit(e.target.value)}
                  />
                </div>

                {canManageEventRetention && (
                  <div className="form inline audit-retention">
                    <label>Retention (days)</label>
                    <input
                      type="number"
                      min="1"
                      max="3650"
                      value={eventRetentionDays}
                      onChange={(e) => setEventRetentionDays(e.target.value)}
                    />
                    <button className="button ghost" onClick={updateEventRetention}>
                      Update retention
                    </button>
                    <div className="status">
                      Last updated: {eventRetention.updatedAt ? new Date(eventRetention.updatedAt).toLocaleString() : '—'}
                    </div>
                    {eventRetentionStatus && <div className="status">{eventRetentionStatus}</div>}
                  </div>
                )}

                {eventLoading ? (
                  <div className="placeholder">Loading retained events...</div>
                ) : (
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>Time</th>
                          <th>Type</th>
                          <th>Device</th>
                          <th>Payload</th>
                        </tr>
                      </thead>
                      <tbody>
                        {eventRows.map((row, idx) => (
                          <tr key={`${row.eventId || row.occurredAt}-${idx}`}>
                            <td>{formatTime(row.occurredAt)}</td>
                            <td>{row.type}</td>
                            <td>{row.deviceId || '—'}</td>
                            <td className="event-payload">
                              <code>{row.payload ? JSON.stringify(row.payload) : '—'}</code>
                            </td>
                          </tr>
                        ))}
                        {eventRows.length === 0 && (
                          <tr>
                            <td colSpan={4}>No retained events found.</td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}

            {logsTab === 'device' && (
              <>
                <div className="form inline">
                  <label>Device ID</label>
                  <select
                    value={logsDeviceId}
                    onChange={(e) => setLogsDeviceId(e.target.value)}
                  >
                    <option value="">Select device…</option>
                    {devices.map((device) => (
                      <option key={device.deviceId} value={device.deviceId}>
                        {device.deviceId} ({device.status || 'unknown'})
                      </option>
                    ))}
                  </select>
                  <label>From</label>
                  <input
                    type="datetime-local"
                    value={logFrom}
                    onChange={(e) => setLogFrom(e.target.value)}
                  />
                  <label>To</label>
                  <input
                    type="datetime-local"
                    value={logTo}
                    onChange={(e) => setLogTo(e.target.value)}
                  />
                  {selectedDeviceId && (
                    <button className="button ghost" onClick={() => setLogsDeviceId(selectedDeviceId)}>
                      Use selected device
                    </button>
                  )}
                  {logsDeviceId && (
                    <button className="button ghost" onClick={() => downloadLogs(logsDeviceId)}>
                      Download CSV
                    </button>
                  )}
                </div>

                {logError && <div className="error">{logError}</div>}
                {logLoading ? (
                  <div className="placeholder">Loading logs...</div>
                ) : (
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>Timestamp</th>
                          <th>Level</th>
                          <th>Component</th>
                          <th>Device</th>
                          <th>Message</th>
                        </tr>
                      </thead>
                      <tbody>
                        {filteredLogs.map((row, idx) => (
                          <tr key={`${row.timestamp}-${idx}`} className={`log-row level-${(row.level || '').toLowerCase()}`}>
                            <td>{row.timestamp}</td>
                            <td>{row.level}</td>
                            <td>{row.component}</td>
                            <td>{row.deviceId}</td>
                            <td>{row.message}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            )}
            {logsTab === 'audit' && canViewAudit && (
              <>
                <div className="form inline">
                  <label>Action</label>
                  <input
                    value={auditAction}
                    onChange={(e) => setAuditAction(e.target.value)}
                    placeholder="artifact.upload"
                  />
                  <label>Actor Type</label>
                  <input
                    value={auditActorType}
                    onChange={(e) => setAuditActorType(e.target.value)}
                    placeholder="user/device/system"
                  />
                  <label>Actor ID</label>
                  <input value={auditActorId} onChange={(e) => setAuditActorId(e.target.value)} />
                  <label>Target Type</label>
                  <input
                    value={auditTargetType}
                    onChange={(e) => setAuditTargetType(e.target.value)}
                    placeholder="device/artifact/group"
                  />
                  <label>Target ID</label>
                  <input value={auditTargetId} onChange={(e) => setAuditTargetId(e.target.value)} />
                  <label>Status</label>
                  <select value={auditStatus} onChange={(e) => setAuditStatus(e.target.value)}>
                    <option value="">All</option>
                    <option value="success">success</option>
                    <option value="error">error</option>
                  </select>
                  <label>From</label>
                  <input
                    type="datetime-local"
                    value={auditFrom}
                    onChange={(e) => setAuditFrom(e.target.value)}
                  />
                  <label>To</label>
                  <input
                    type="datetime-local"
                    value={auditTo}
                    onChange={(e) => setAuditTo(e.target.value)}
                  />
                  <label>Limit</label>
                  <input
                    type="number"
                    min="1"
                    max="5000"
                    value={auditLimit}
                    onChange={(e) => setAuditLimit(e.target.value)}
                  />
                </div>

                {canManageAuditRetention && (
                  <div className="form inline audit-retention">
                    <label>Retention (days)</label>
                    <input
                      type="number"
                      min="1"
                      max="3650"
                      value={auditRetentionDays}
                      onChange={(e) => setAuditRetentionDays(e.target.value)}
                    />
                    <button className="button ghost" onClick={updateAuditRetention}>
                      Update retention
                    </button>
                    <div className="status">
                      Last updated: {auditRetention.updatedAt ? new Date(auditRetention.updatedAt).toLocaleString() : '—'}
                    </div>
                    {auditRetentionStatus && <div className="status">{auditRetentionStatus}</div>}
                  </div>
                )}

                {auditError && <div className="error">{auditError}</div>}
                {auditLoading ? (
                  <div className="placeholder">Loading audit events...</div>
                ) : (
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>Time</th>
                          <th>Action</th>
                          <th>Actor</th>
                          <th>Target</th>
                          <th>Status</th>
                          <th>Error</th>
                        </tr>
                      </thead>
                      <tbody>
                        {auditRows.map((row, idx) => (
                          <tr key={`${row.eventId}-${idx}`} className={`audit-row status-${row.status || 'success'}`}>
                            <td>{formatTime(row.occurredAt)}</td>
                            <td>{row.action}</td>
                            <td>
                              {row.actorEmail || row.actorId || row.actorType || '—'}
                            </td>
                            <td>
                              {row.targetType && row.targetId
                                ? `${row.targetType}:${row.targetId}`
                                : row.targetType || row.targetId || '—'}
                            </td>
                            <td>{row.status}</td>
                            <td>{row.error || '—'}</td>
                          </tr>
                        ))}
                        {auditRows.length === 0 && (
                          <tr>
                            <td colSpan={6}>No audit events found.</td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            )}
          </section>
        )}

        {view === 'security' && (
          <section id="security" className="card settings-card">
            <div className="section-header settings-header">
              <h2>Security</h2>
              <div className="settings-toolbar">
                <button className="button ghost" onClick={loadRotationStatus}>Refresh rotation</button>
                {canManagePendingEnrollments && (
                  <button className="button ghost" onClick={loadPendingEnrollments}>Refresh pending enrollments</button>
                )}
                {authStatus.enabled && authToken && canManageUsers && (
                  <button className="button ghost" onClick={loadUsers}>Refresh users</button>
                )}
              </div>
            </div>

            <div className="settings-stack">
              <div className="settings-section">
                <div className="settings-title">Authentication</div>
                {!authStatus.enabled && (
                  <div className="placeholder">Auth is disabled on the control-plane.</div>
                )}
                {authUser ? (
                  <div className="detail-grid">
                    <div>
                      <div className="detail-label">User</div>
                      <div className="detail-value">{authUser.email}</div>
                    </div>
                    <div>
                      <div className="detail-label">Roles</div>
                      <div className="detail-value">{(authUser.roles || []).join(', ') || '—'}</div>
                    </div>
                    <div className="full">
                      <button className="button ghost" onClick={doLogout}>
                        Sign out
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="form compact">
                    <div className="field">
                      <label>Email</label>
                      <input
                        value={loginForm.email}
                        onChange={(e) => setLoginForm((prev) => ({ ...prev, email: e.target.value }))}
                        placeholder="admin@example.com"
                      />
                    </div>
                    <div className="field">
                      <label>Password</label>
                      <input
                        type="password"
                        value={loginForm.password}
                        onChange={(e) => setLoginForm((prev) => ({ ...prev, password: e.target.value }))}
                      />
                    </div>
                    <div className="field actions">
                      <button className="button ghost" onClick={doLogin}>
                        Sign in
                      </button>
                    </div>
                  </div>
                )}
                {authError && <div className="error">{authError}</div>}
                {loginStatus && <div className="status">{loginStatus}</div>}
              </div>

              <div className="settings-section">
                <div className="settings-title">Local Users</div>
                {!authStatus.enabled ? (
                  <div className="placeholder">Enable AUTH_MODE=local to manage users.</div>
                ) : !authToken ? (
                  <div className="placeholder">Sign in to manage local users.</div>
                ) : !canManageUsers ? (
                  <div className="placeholder">Admin role required to manage users.</div>
                ) : (
                  <>
                    <div className="form compact">
                      <div className="field">
                        <label>Email</label>
                        <input
                          value={userForm.email}
                          onChange={(e) => setUserForm((prev) => ({ ...prev, email: e.target.value }))}
                          placeholder="user@example.com"
                        />
                      </div>
                      <div className="field">
                        <label>Password</label>
                        <input
                          type="password"
                          value={userForm.password}
                          onChange={(e) => setUserForm((prev) => ({ ...prev, password: e.target.value }))}
                        />
                      </div>
                      <div className="field">
                        <label>Display Name</label>
                        <input
                          value={userForm.displayName}
                          onChange={(e) => setUserForm((prev) => ({ ...prev, displayName: e.target.value }))}
                        />
                      </div>
                      <div className="field">
                        <label>Roles</label>
                        <div className="inline-row">
                          {['viewer', 'operator', 'admin'].map((role) => (
                            <label key={role} className="chip">
                              <input
                                type="checkbox"
                                checked={userForm.roles.includes(role)}
                                onChange={() => toggleRole(role)}
                              />
                              {role}
                            </label>
                          ))}
                        </div>
                      </div>
                      <div className="field actions">
                        <button className="button ghost" onClick={submitUser}>
                          Create user
                        </button>
                      </div>
                    </div>
                    {usersError && <div className="error">{usersError}</div>}
                    {usersStatus && <div className="status">{usersStatus}</div>}
                    <div className="table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>Email</th>
                            <th>Display Name</th>
                            <th>Roles</th>
                            <th>Disabled</th>
                            <th>Created</th>
                          </tr>
                        </thead>
                        <tbody>
                          {users.map((user) => (
                            <tr key={user.userId}>
                              <td>{user.email}</td>
                              <td>{user.displayName || '—'}</td>
                              <td>{(user.roles || []).join(', ') || '—'}</td>
                              <td>{user.disabled ? 'yes' : 'no'}</td>
                              <td>{user.createdAt ? new Date(user.createdAt).toLocaleString() : '—'}</td>
                            </tr>
                          ))}
                          {users.length === 0 && (
                            <tr>
                              <td colSpan={5}>No users found.</td>
                            </tr>
                          )}
                        </tbody>
                      </table>
                    </div>

                    <div className="form compact">
                      <div className="field">
                        <label>Invite Email (optional)</label>
                        <input
                          value={voucherForm.email}
                          onChange={(e) => setVoucherForm((prev) => ({ ...prev, email: e.target.value }))}
                          placeholder="user@example.com"
                        />
                      </div>
                      <div className="field">
                        <label>TTL (hours)</label>
                        <input
                          type="number"
                          min="1"
                          max="720"
                          value={voucherForm.ttlHours}
                          onChange={(e) => setVoucherForm((prev) => ({ ...prev, ttlHours: e.target.value }))}
                        />
                      </div>
                      <div className="field">
                        <label>Roles</label>
                        <div className="inline-row">
                          {['viewer', 'operator', 'admin'].map((role) => (
                            <label key={role} className="chip">
                              <input
                                type="checkbox"
                                checked={voucherForm.roles.includes(role)}
                                onChange={() => toggleVoucherRole(role)}
                              />
                              {role}
                            </label>
                          ))}
                        </div>
                      </div>
                      <div className="field actions">
                        <button className="button ghost" onClick={submitVoucher}>
                          Create voucher
                        </button>
                        {voucherStatus && <div className="status">{voucherStatus}</div>}
                      </div>
                    </div>
                    {voucherToken && (
                      <div className="form compact">
                        <div className="field">
                          <label>Voucher Token</label>
                          <input readOnly value={voucherToken} />
                        </div>
                      </div>
                    )}
                  </>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-title">Enrollment Profiles</div>
                {!canManagePendingEnrollments ? (
                  <div className="placeholder">Operator role required to manage enrollment profiles.</div>
                ) : (
                  <>
                    <div className="inline-row">
                      <button className="button ghost" onClick={handleOpenCreateEnrollmentProfile}>
                        New profile
                      </button>
                      <button className="button ghost" onClick={loadEnrollmentProfiles}>
                        Refresh profiles
                      </button>
                    </div>
                    {enrollmentProfilesStatus && <div className="status">{enrollmentProfilesStatus}</div>}
                    {enrollmentProfilesError && <div className="error">{enrollmentProfilesError}</div>}
                    {latestEnrollmentProfileToken && (
                      <div className="form compact">
                        <div className="field">
                          <label>Latest Bootstrap Token</label>
                          <input readOnly value={latestEnrollmentProfileToken} />
                        </div>
                        <div className="field actions">
                          <button className="button ghost" onClick={handleCopyEnrollmentProfileToken}>
                            Copy token
                          </button>
                          <button className="button ghost" onClick={handleDownloadEnrollmentProfileToken}>
                            Download token
                          </button>
                        </div>
                      </div>
                    )}
                    {enrollmentProfilesLoading ? (
                      <div className="placeholder">Loading enrollment profiles...</div>
                    ) : (
                      <div className="table-wrap">
                        <table>
                          <thead>
                            <tr>
                              <th>Name</th>
                              <th>Labels</th>
                              <th>Uses</th>
                              <th>Approval</th>
                              <th>Challenge</th>
                              <th>Delay</th>
                              <th>Unsigned HW</th>
                              <th>Expires</th>
                              <th>Status</th>
                              <th>Actions</th>
                            </tr>
                          </thead>
                          <tbody>
                            {enrollmentProfiles.map((profile) => (
                              <tr key={profile.profileId}>
                                <td>{profile.name}</td>
                                <td className="mono">{formatObjectSummary(profile.defaultLabels)}</td>
                                <td>{profile.maxUses > 0 ? `${profile.uses}/${profile.maxUses}` : `${profile.uses}/unlimited`}</td>
                                <td>{profile.requireApproval ? 'required' : 'not required'}</td>
                                <td>{profile.challengeEnabled ? profile.challengeHint || 'enabled' : 'disabled'}</td>
                                <td>{profile.approvalDelaySec > 0 ? formatDurationSeconds(profile.approvalDelaySec) : 'none'}</td>
                                <td>{profile.allowUnsignedHardwareIdentity ? 'allowed' : 'blocked'}</td>
                                <td>{formatTime(profile.expiresAt)}</td>
                                <td>{profile.disabled ? 'disabled' : 'active'}</td>
                                <td>
                                  <div className="inline-row">
                                    <button
                                      className="button ghost"
                                      onClick={() => handleStartEditEnrollmentProfile(profile)}
                                    >
                                      Edit
                                    </button>
                                    <button
                                      className="button ghost"
                                      onClick={() => handleRotateEnrollmentProfile(profile.profileId, profile.name)}
                                    >
                                      Rotate Token
                                    </button>
                                    <button
                                      className="button ghost"
                                      onClick={() => handleSetEnrollmentProfileDisabled(profile.profileId, !profile.disabled)}
                                    >
                                      {profile.disabled ? 'Enable' : 'Disable'}
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                            {enrollmentProfiles.length === 0 && (
                              <tr>
                                <td colSpan={10}>No enrollment profiles.</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-title">Pending Enrollments</div>
                {!canManagePendingEnrollments ? (
                  <div className="placeholder">Operator role required to review pending enrollments.</div>
                ) : (
                  <>
                    <div className="inline-row">
                      <label className="chip">
                        <span>Filter</span>
                        <select value={pendingEnrollmentsFilter} onChange={(e) => setPendingEnrollmentsFilter(e.target.value)}>
                          <option value="pending">pending</option>
                          <option value="approved">approved</option>
                          <option value="conflict">conflict</option>
                          <option value="denied">denied</option>
                          <option value="expired">expired</option>
                          <option value="issued">issued</option>
                          <option value="all">all</option>
                        </select>
                      </label>
                      <button className="button ghost" onClick={loadPendingEnrollments}>
                        Refresh
                      </button>
                    </div>
                    {pendingEnrollmentsStatus && <div className="status">{pendingEnrollmentsStatus}</div>}
                    {pendingEnrollmentsError && <div className="error">{pendingEnrollmentsError}</div>}
                    {pendingEnrollmentsLoading ? (
                      <div className="placeholder">Loading pending enrollments...</div>
                    ) : (
                      <div className="table-wrap">
                        <table>
                          <thead>
                            <tr>
                              <th>Request ID</th>
                              <th>Status</th>
                              <th>Agent</th>
                              <th>Hardware ID</th>
                              <th>Source IP</th>
                              <th>Reason</th>
                              <th>Created</th>
                              <th>Expires</th>
                              <th>Approve After</th>
                              <th>Actions</th>
                            </tr>
                          </thead>
                          <tbody>
                            {pendingEnrollments.map((item) => {
                              const approvalDt = item.approvalAvailableAt ? new Date(item.approvalAvailableAt) : null
                              const approvalPending = Boolean(
                                item.status === 'pending'
                                  && approvalDt
                                  && !Number.isNaN(approvalDt.getTime())
                                  && approvalDt.getTime() > Date.now(),
                              )
                              const approvalDelayLabel = approvalPending
                                ? `${formatDurationSeconds((approvalDt.getTime() - Date.now()) / 1000)} (${formatTime(item.approvalAvailableAt)})`
                                : formatTime(item.approvalAvailableAt)
                              return (
                                <tr key={item.requestId}>
                                  <td className="mono">{item.requestId}</td>
                                  <td>{item.status || '—'}</td>
                                  <td>{item.agentVersion || '—'}</td>
                                  <td className="mono">{item.hardwareId || '—'}</td>
                                  <td className="mono">{item.sourceIp || '—'}</td>
                                  <td>{item.deniedReason || '—'}</td>
                                  <td>{formatTime(item.createdAt)}</td>
                                  <td>{formatTime(item.expiresAt)}</td>
                                  <td>{approvalPending ? approvalDelayLabel : approvalDelayLabel || '—'}</td>
                                  <td>
                                    {item.status === 'pending' ? (
                                      <div className="inline-row">
                                        <button
                                          className="button ghost"
                                          disabled={approvalPending}
                                          title={approvalPending ? `Approval available after ${formatTime(item.approvalAvailableAt)}` : ''}
                                          onClick={() => handleApprovePendingEnrollment(item.requestId)}
                                        >
                                          {approvalPending ? 'Throttled' : 'Approve'}
                                        </button>
                                        <button className="button ghost" onClick={() => handleDenyPendingEnrollment(item.requestId)}>
                                          Deny
                                        </button>
                                      </div>
                                    ) : ['denied', 'conflict', 'expired'].includes(item.status) ? (
                                      <button className="button ghost" onClick={() => handleResetPendingEnrollment(item.requestId)}>
                                        Reset
                                      </button>
                                    ) : (
                                      '—'
                                    )}
                                  </td>
                                </tr>
                              )
                            })}
                            {pendingEnrollments.length === 0 && (
                              <tr>
                                <td colSpan={10}>No enrollment requests.</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-title">CA Rotation</div>
                <div className="inline-row">
                  <button className="button ghost" onClick={loadRotationStatus}>
                    Refresh
                  </button>
                  {canRotate && (
                    <>
                      <button className="button" onClick={handleRotateRotation}>
                        Rotate CA
                      </button>
                      <button className="button ghost" onClick={handleReloadRotation}>
                        Reload CA files
                      </button>
                      <button
                        className="button ghost"
                        onClick={handleCleanupRotation}
                        disabled={!rotationStatus?.cleanup?.eligible}
                      >
                        Cleanup old CA
                      </button>
                    </>
                  )}
                </div>
                {rotationMessage && <div className="status">{rotationMessage}</div>}
                {rotationError && <div className="error">{rotationError}</div>}
                {rotationLoading ? (
                  <div className="placeholder">Loading rotation status...</div>
                ) : rotationStatus ? (
                  <div className="rotation-grid">
                    <div className="rotation-card">
                      <div className="rotation-title">Active CA</div>
                      <div className="rotation-row">
                        <span>Path</span>
                        <span className="mono">{rotationStatus.activeCa?.path || '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Subject</span>
                        <span>{rotationStatus.activeCa?.subject || '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Fingerprint</span>
                        <span className="mono">{rotationStatus.activeCa?.fingerprint || '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Valid</span>
                        <span>
                          {rotationStatus.activeCa?.notBefore ? new Date(rotationStatus.activeCa.notBefore).toLocaleDateString() : '—'}
                          {' → '}
                          {rotationStatus.activeCa?.notAfter ? new Date(rotationStatus.activeCa.notAfter).toLocaleDateString() : '—'}
                        </span>
                      </div>
                    </div>

                    <div className="rotation-card">
                      <div className="rotation-title">Client CA Bundle</div>
                      <div className="rotation-row">
                        <span>Path</span>
                        <span className="mono">{rotationStatus.clientCa?.path || '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Certs</span>
                        <span>{rotationStatus.clientCa?.certCount ?? '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Contains active</span>
                        <span>{rotationStatus.clientCa?.containsActive ? 'Yes' : 'No'}</span>
                      </div>
                    </div>

                    <div className="rotation-card">
                      <div className="rotation-title">Device Coverage</div>
                      <div className="rotation-row">
                        <span>Total</span>
                        <span>{rotationStatus.deviceCounts?.total ?? '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Active CA</span>
                        <span>{rotationStatus.deviceCounts?.active ?? '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Needs reenroll</span>
                        <span>{rotationStatus.deviceCounts?.needsReenroll ?? '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Unknown</span>
                        <span>{rotationStatus.deviceCounts?.unknown ?? '—'}</span>
                      </div>
                      <div className="rotation-note">
                        Re-enroll updates existing devices and does not consume additional license slots.
                      </div>
                    </div>

                    <div className="rotation-card">
                      <div className="rotation-title">Cleanup Status</div>
                      <div className="rotation-row">
                        <span>Eligible</span>
                        <span>{rotationStatus.cleanup?.eligible ? 'Yes' : 'No'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Reason</span>
                        <span>{rotationStatus.cleanup?.reason || '—'}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Rotated</span>
                        <span>
                          {rotationStatus.cleanup?.rotatedAt
                            ? new Date(rotationStatus.cleanup.rotatedAt).toLocaleString()
                            : '—'}
                        </span>
                      </div>
                      <div className="rotation-row">
                        <span>Grace remaining</span>
                        <span>{formatDurationSeconds(rotationStatus.cleanup?.graceRemainingSec || 0)}</span>
                      </div>
                      <div className="rotation-row">
                        <span>Grace deadline</span>
                        <span>
                          {rotationStatus.cleanup?.graceDeadline
                            ? new Date(rotationStatus.cleanup.graceDeadline).toLocaleString()
                            : '—'}
                        </span>
                      </div>
                      <div className="rotation-row">
                        <span>Cleaned</span>
                        <span>
                          {rotationStatus.cleanup?.cleanedAt
                            ? new Date(rotationStatus.cleanup.cleanedAt).toLocaleString()
                            : '—'}
                        </span>
                      </div>
                      {rotationStatus.cleanup?.previousFingerprint && (
                        <div className="rotation-row">
                          <span>Old CA</span>
                          <span className="mono">{rotationStatus.cleanup.previousFingerprint}</span>
                        </div>
                      )}
                    </div>
                  </div>
                ) : (
                  <div className="placeholder">No rotation status yet.</div>
                )}
              </div>
            </div>
          </section>
        )}

        {view === 'settings' && (
          <section id="settings" className="card settings-card">
            <div className="section-header settings-header">
              <h2>Settings</h2>
              <div className="settings-toolbar">
                <button className="button ghost" onClick={loadMaintenance}>Refresh maintenance</button>
                <button className="button ghost" onClick={() => { loadArtifactLifecyclePolicy(); loadArtifactLifecycleStatus() }}>
                  Refresh lifecycle
                </button>
                <button className="button ghost" onClick={loadReleaseAutoUpdate}>Refresh auto-update</button>
                <button className="button ghost" onClick={loadUpgrade}>Refresh upgrade</button>
                <button className="button ghost" onClick={loadUpgradeAvailable}>Refresh updates</button>
              </div>
            </div>

            <div className="settings-stack">
              <div className="settings-section">
                <div className="settings-title">Maintenance</div>
                <div className="detail-grid">
                  <div>
                    <div className="detail-label">Maintenance</div>
                    <div className="detail-value">{maintenance.enabled ? 'enabled' : 'disabled'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Message</div>
                    <div className="detail-value">{maintenance.message || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Updated</div>
                    <div className="detail-value">{maintenance.updatedAt ? new Date(maintenance.updatedAt).toLocaleString() : '—'}</div>
                  </div>
                  <div className="full">
                    <button className="button ghost" onClick={toggleMaintenance} disabled={!canManageMaintenance}>
                      {maintenance.enabled ? 'Disable maintenance' : 'Enable maintenance'}
                    </button>
                  </div>
                </div>
              </div>

              <div className="settings-section">
                <div className="settings-title">Artifact Lifecycle</div>
                <div className="artifact-lifecycle-toolbar">
                  <div className="inline-row">
                    <label className="artifact-lifecycle-label">Deprecated retention (days)</label>
                    <input
                      type="number"
                      min={1}
                      step={1}
                      value={artifactLifecyclePolicyInput}
                      onChange={(e) => setArtifactLifecyclePolicyInput(e.target.value)}
                      disabled={!canManageArtifactLifecycle}
                    />
                    <button
                      className="button ghost"
                      onClick={handleSaveArtifactLifecyclePolicy}
                      disabled={!canManageArtifactLifecycle}
                    >
                      Save policy
                    </button>
                  </div>
                  <div className="hint">
                    Deprecated artifacts are eligible for prune after the configured retention window, if not referenced.
                  </div>
                  <div className="detail-note">
                    Policy updated: {formatTime(artifactLifecyclePolicy.updatedAt)}
                  </div>
                  <div className="detail-note">
                    Auto prune: {artifactLifecycleStatus.enabled ? 'enabled' : 'disabled'}
                    {artifactLifecycleStatus.intervalSeconds > 0
                      ? ` every ${formatDurationSeconds(artifactLifecycleStatus.intervalSeconds)}`
                      : ''}
                    {artifactLifecycleStatus.running ? ' (running)' : ''}
                  </div>
                  {artifactLifecycleStatus.lastRun && (
                    <div className="detail-note">
                      Last run: {formatTime(artifactLifecycleStatus.lastRun.finishedAt || artifactLifecycleStatus.lastRun.cutoffUtc)} ·
                      deleted {artifactLifecycleStatus.lastRun.deletedNum || 0} ·
                      skipped {artifactLifecycleStatus.lastRun.skippedNum || 0}
                      {artifactLifecycleStatus.lastRun.error ? ` · error: ${artifactLifecycleStatus.lastRun.error}` : ''}
                    </div>
                  )}
                  {artifactLifecycleStatus.alerts.length > 0 && (
                    <div className="artifact-lifecycle-alerts">
                      {artifactLifecycleStatus.alerts.map((alert) => (
                        <span key={alert.code} className={`pill artifact-alert ${alert.severity || 'warning'}`}>
                          {alert.message}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </div>

              <div className="settings-section">
                <div className="settings-title">Release Auto-Update</div>
                <div className="detail-grid">
                  <div>
                    <div className="detail-label">Enabled (default)</div>
                    <label className="inline-toggle">
                      <input
                        type="checkbox"
                        checked={Boolean(releaseAutoUpdate.enabled)}
                        onChange={(e) => {
                          const enabled = e.target.checked
                          setReleaseAutoUpdate((prev) => ({ ...prev, enabled }))
                        }}
                        disabled={!canManageReleaseAutoUpdate}
                      />
                      {releaseAutoUpdate.enabled ? 'on' : 'off'}
                    </label>
                  </div>
                  <div>
                    <div className="detail-label">Allow unsigned</div>
                    <label className="inline-toggle">
                      <input
                        type="checkbox"
                        checked={Boolean(releaseAutoUpdate.allowUnsigned)}
                        onChange={(e) => {
                          const allowUnsigned = e.target.checked
                          setReleaseAutoUpdate((prev) => ({ ...prev, allowUnsigned }))
                        }}
                        disabled={!canManageReleaseAutoUpdate}
                      />
                      {releaseAutoUpdate.allowUnsigned ? 'yes' : 'no'}
                    </label>
                  </div>
                  <div>
                    <div className="detail-label">Interval</div>
                    <div className="detail-value">
                      {releaseAutoUpdate.intervalSeconds > 0
                        ? formatDurationSeconds(releaseAutoUpdate.intervalSeconds)
                        : '—'}
                    </div>
                  </div>
                  <div>
                    <div className="detail-label">Running</div>
                    <div className="detail-value">{releaseAutoUpdate.running ? 'yes' : 'no'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Updated</div>
                    <div className="detail-value">
                      {releaseAutoUpdate.updatedAt
                        ? new Date(releaseAutoUpdate.updatedAt).toLocaleString()
                        : '—'}
                    </div>
                  </div>
                  <div>
                    <div className="detail-label">Updated by</div>
                    <div className="detail-value">{releaseAutoUpdate.updatedByUserId || '—'}</div>
                  </div>
                  <div className="full inline-row">
                    <button
                      className="button ghost"
                      onClick={handleSaveReleaseAutoUpdateSettings}
                      disabled={!canManageReleaseAutoUpdate || releaseAutoUpdateSaving}
                    >
                      Save auto-update settings
                    </button>
                    <button
                      className="button ghost"
                      onClick={handleRunReleaseAutoUpdate}
                      disabled={!canManageReleaseAutoUpdate || releaseAutoUpdateSaving || releaseAutoUpdate.running}
                    >
                      Run now
                    </button>
                  </div>
                </div>
                {releaseAutoUpdate.lastRun && (
                  <div className="detail-note">
                    Last run: {formatTime(releaseAutoUpdate.lastRun.finishedAt || releaseAutoUpdate.lastRun.startedAt)} ·
                    trigger {releaseAutoUpdate.lastRun.trigger || '—'} ·
                    updated components {Number(releaseAutoUpdate.lastRun.componentsUpdated || 0)} ·
                    groups {Number(releaseAutoUpdate.lastRun.groupsUpdated || 0)} ·
                    devices {Number(releaseAutoUpdate.lastRun.devicesUpdated || 0)}
                    {releaseAutoUpdate.lastRun.error ? ` · error: ${releaseAutoUpdate.lastRun.error}` : ''}
                  </div>
                )}
                {releaseAutoUpdateStatus && <div className="status">{releaseAutoUpdateStatus}</div>}
              </div>

              <div className="settings-section">
                <div className="settings-title">Backups</div>
                {!canViewBackups ? (
                  <div className="placeholder">Admin role required to view backups and restore status.</div>
                ) : !backupStatus.enabled ? (
                  <div className="placeholder">Backup runner not configured.</div>
                ) : (
                  <>
                    <div className="detail-grid">
                      <div>
                        <div className="detail-label">Backup status</div>
                        <div className="detail-value">{backupStatus.state || 'idle'}</div>
                      </div>
                      <div>
                        <div className="detail-label">Restore status</div>
                        <div className="detail-value">{restoreStatus.state || 'idle'}</div>
                      </div>
                      <div>
                        <div className="detail-label">Log</div>
                        <div className="detail-value">{backupStatus.logPath || restoreStatus.logPath || '—'}</div>
                      </div>
                      <div className="full actions">
                        <button className="button ghost" onClick={loadBackups}>
                          Refresh backups
                        </button>
                        <button className="button" onClick={handleStartBackup} disabled={!canManageBackups}>
                          Create backup
                        </button>
                      </div>
                    </div>
                    <div className="form compact">
                      <div className="field">
                        <label>Restore backup</label>
                        <select value={selectedBackupId} onChange={(e) => setSelectedBackupId(e.target.value)}>
                          <option value="">Select backup</option>
                          {backups.map((item) => (
                            <option key={item.id} value={item.id}>
                              {item.id}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className="field actions">
                        <button className="button ghost" onClick={handleRestore} disabled={!canManageBackups || !maintenance.enabled}>
                          Restore + wipe
                        </button>
                        {!maintenance.enabled && (
                          <div className="status">Enable maintenance before restore.</div>
                        )}
                      </div>
                    </div>
                    {backupError && <div className="error">{backupError}</div>}
                    {backupMessage && <div className="status">{backupMessage}</div>}
                    <div className="table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>ID</th>
                            <th>Created</th>
                            <th>Size</th>
                          </tr>
                        </thead>
                        <tbody>
                          {backups.map((item) => (
                            <tr key={item.id}>
                              <td>{item.id}</td>
                              <td>{item.createdAt ? new Date(item.createdAt).toLocaleString() : '—'}</td>
                              <td>{item.sizeBytes ? `${(item.sizeBytes / (1024 * 1024)).toFixed(1)} MB` : '—'}</td>
                            </tr>
                          ))}
                          {backups.length === 0 && (
                            <tr>
                              <td colSpan={3}>No backups found.</td>
                            </tr>
                          )}
                        </tbody>
                      </table>
                    </div>
                  </>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-title">Upgrade</div>
                {!upgrade.enabled ? (
                  <div className="placeholder">Upgrade runner not configured.</div>
                ) : (
                  <>
                    <div className="detail-grid">
                      <div>
                        <div className="detail-label">Maintenance</div>
                        <div className="detail-value">{maintenance.enabled ? 'enabled' : 'disabled'}</div>
                      </div>
                      <div>
                        <div className="detail-label">Updates</div>
                        <div className="detail-value">{upgradeAvailable.available ? 'available' : 'none'}</div>
                      </div>
                      <div>
                        <div className="detail-label">Preflight</div>
                        <div className="detail-value">{preflightOk ? 'ok' : 'not run / failed'}</div>
                      </div>
                      <div>
                        <div className="detail-label">Last Preflight</div>
                        <div className="detail-value">
                          {upgradePreflight.timestamp ? new Date(upgradePreflight.timestamp).toLocaleString() : '—'}
                        </div>
                      </div>
                      <div>
                        <div className="detail-label">State</div>
                        <div className="detail-value">{upgrade.state || 'idle'}</div>
                      </div>
                      <div>
                        <div className="detail-label">Running</div>
                        <div className="detail-value">{upgrade.running ? 'yes' : 'no'}</div>
                      </div>
                      <div className="full">
                        <div className="detail-label">Log Path</div>
                        <div className="detail-value">{upgrade.logPath || '—'}</div>
                      </div>
                      {upgrade.error && (
                        <div className="full">
                          <div className="detail-label">Error</div>
                          <div className="detail-value">{upgrade.error}</div>
                        </div>
                      )}
                    </div>

                    <div className="inline-row">
                      <button className="button ghost" onClick={loadUpgradeAvailable}>
                        Refresh updates
                      </button>
                      <button className="button ghost" onClick={loadUpgradePreflight}>
                        Run preflight
                      </button>
                      {canApplyUpgrade && maintenance.enabled && upgrade.enabled && (
                        <button
                          className="button ghost"
                          onClick={startUpgrade}
                          disabled={upgrade.running || !upgradeReady}
                        >
                          {upgrade.running ? 'Applying update…' : 'Apply update'}
                        </button>
                      )}
                    </div>
                    {upgradeAvailableError && <div className="error">{upgradeAvailableError}</div>}
                    {upgradePreflightStatus && <div className="status">{upgradePreflightStatus}</div>}
                    {upgradePreflightError && <div className="error">{upgradePreflightError}</div>}

                    {!upgradeAvailable.available ? (
                      <div className="placeholder">
                        No updates found{upgradeAvailable.updatesDir ? ` in ${upgradeAvailable.updatesDir}.` : '.'}
                      </div>
                    ) : (
                      <div className="detail-grid">
                        <div>
                          <div className="detail-label">Latest</div>
                          <div className="detail-value">{upgradeAvailable.latest}</div>
                        </div>
                        <div>
                          <div className="detail-label">Updates Dir</div>
                          <div className="detail-value">{upgradeAvailable.updatesDir || '—'}</div>
                        </div>
                        <div className="full">
                          <div className="detail-label">Bundles</div>
                          <div className="detail-value">{(upgradeAvailable.bundles || []).join(', ')}</div>
                        </div>
                      </div>
                    )}

                    <div className="preflight-list">
                      {(upgradePreflight.checks || []).map((check) => (
                        <div key={check.name} className={`preflight-item ${check.status}`}>
                          <div className="preflight-title">
                            <span className={`preflight-badge ${check.status}`}>{check.status}</span>
                            {check.name}
                          </div>
                          <div className="preflight-msg">{check.message}</div>
                        </div>
                      ))}
                      {(!upgradePreflight.checks || upgradePreflight.checks.length === 0) && (
                        <div className="placeholder">No preflight results yet.</div>
                      )}
                    </div>
                  </>
                )}
              </div>
            </div>
          </section>
        )}
      </main>

      {deviceDrawerOpen && (
        <div className="drawer-backdrop" onClick={() => setDeviceDrawerOpen(false)}>
          <div
            className="drawer"
            style={{ width: `min(${drawerWidth}px, 90vw)` }}
            onClick={(e) => e.stopPropagation()}
          >
            <div
              className="drawer-handle"
              onPointerDown={(event) => {
                drawerResizingRef.current = true
                event.preventDefault()
              }}
            />
            <div className="drawer-header">
              <div>
                <div className="detail-label">Device</div>
                <div className="detail-value">{selectedDeviceId || '—'}</div>
              </div>
              <button className="button ghost" onClick={() => setDeviceDrawerOpen(false)}>
                Close
              </button>
            </div>
            {deviceDetailError && <div className="error">{deviceDetailError}</div>}
            {!selectedDeviceId && <div className="placeholder">Select a device to view details.</div>}
            {selectedDeviceId && !deviceDetail && <div className="placeholder">Loading device detail...</div>}
            {deviceDetail && (
              <div className="drawer-content">
                <div className="detail-grid">
                  <div>
                    <div className="detail-label">Status</div>
                    <div className="detail-value">{deviceDetail.status || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Last Seen</div>
                    <div className="detail-value">{deviceDetail.lastSeen || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Current Version</div>
                    <div className="detail-value">{deviceDetail.current?.softwareVersion || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Config Rev</div>
                    <div className="detail-value">{deviceDetail.current?.configRev || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Last Apply Status</div>
                    <div className="detail-value">{deviceDetail.current?.lastApplyStatus || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Last Apply Time</div>
                    <div className="detail-value">{deviceDetail.current?.lastApplyAt || '—'}</div>
                  </div>
                  <div>
                    <div className="detail-label">Last Applied Artifact</div>
                    <div className="detail-value">
                      {lastAppliedArtifact
                        ? `${lastAppliedArtifact.name} v${lastAppliedArtifact.version}`
                        : (deviceDetail.current?.lastApplyArtifactId || '—')}
                    </div>
                    {deviceDetail.current?.lastApplyArtifactId && !lastAppliedArtifact && (
                      <div className="detail-note mono">{deviceDetail.current.lastApplyArtifactId}</div>
                    )}
                    {deviceDetail.current?.lastApplyError && (
                      <div className="detail-note">{deviceDetail.current.lastApplyError}</div>
                    )}
                  </div>
                  <div>
                    <div className="detail-label">Last Pre-apply</div>
                    <div className="detail-value">
                      <span className={`pill preapply ${preApplyBadgeClass}`}>
                        {deviceDetail.current?.lastPreApplyStatus || '—'}
                      </span>
                    </div>
                    {deviceDetail.current?.lastPreApplyError && (
                      <div className="detail-note">{deviceDetail.current?.lastPreApplyError}</div>
                    )}
                  </div>
                </div>

                {deviceDetail.current?.components && Object.keys(deviceDetail.current.components).some((key) => key !== 'app_bundle') && (
                  <div className="component-table">
                    <h4>Components</h4>
                    <table>
                      <thead>
                        <tr>
                          <th>Component</th>
                          <th>Current</th>
                          <th>Last Apply</th>
                          <th>Artifact</th>
                        </tr>
                      </thead>
                      <tbody>
                        {Object.entries(deviceDetail.current.components)
                          .filter(([key]) => key !== 'app_bundle')
                          .map(([key, comp]) => (
                          <tr key={key}>
                            <td>{key}</td>
                            <td>{comp.currentVersion || '—'}</td>
                            <td>
                              <span className={`pill ${comp.lastApplyStatus === 'error' ? 'error' : 'success'}`}>
                                {comp.lastApplyStatus || '—'}
                              </span>
                              {comp.lastApplyAt && <div className="detail-note">{comp.lastApplyAt}</div>}
                            </td>
                            <td>{comp.lastApplyArtifactId || '—'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                <form
                  className="form"
                  onSubmit={(e) => {
                    handleDesiredDevice(e)
                  }}
                >
                  <h3>Desired State</h3>
                  {desiredError && <div className="error">{desiredError}</div>}
                  {!canManageDesiredState && (
                    <div className="hint">Operator role required to edit desired state.</div>
                  )}
                  <fieldset disabled={!canManageDesiredState} style={{ border: 0, margin: 0, padding: 0, minWidth: 0 }}>
                    <label>Device ID</label>
                    <input value={deviceForm.deviceId} readOnly />
                    <label>Components</label>
                    <div className="component-editor">
                      {deviceForm.components.map((row, idx) => {
                        const locked = Boolean(row.locked)
                        const type = normalizeArtifactType(row.artifactType)
                        const groupsForType = filterArtifactGroupsByType(activeArtifactGroups, type)
                        const allGroupsForType = filterArtifactGroupsByType(artifactGroups, type)
                        const selected = artifacts.find((a) => a.artifactId === row.artifactId)
                        const selectedGroup = selected
                          ? (groupsForType.find((g) => g.name === selected.name) ||
                            allGroupsForType.find((g) => g.name === selected.name))
                          : null
                        return (
                          <div className="component-row" key={row.id || `${row.key}-${idx}`}>
                            <div className="inline-row">
                              <input
                                value={row.key}
                                onChange={(e) => updateDeviceComponent(idx, { key: e.target.value })}
                                placeholder="component key (e.g. app:customer)"
                                disabled={locked}
                              />
                              <input
                                list="artifact-types"
                                value={row.artifactType}
                                onChange={(e) => updateDeviceComponent(idx, { artifactType: e.target.value })}
                                placeholder="artifact type"
                                disabled={locked}
                              />
                              <select
                                value={row.autoTrackMode || 'inherit'}
                                onChange={(e) => updateDeviceComponent(idx, { autoTrackMode: e.target.value })}
                                disabled={locked}
                              >
                                {autoTrackModes.map((mode) => (
                                  <option key={mode.id} value={mode.id}>{`Auto ${mode.label}`}</option>
                                ))}
                              </select>
                              <label className="inline-toggle">
                                <input
                                  type="checkbox"
                                  checked={locked}
                                  onChange={(e) => updateDeviceComponent(idx, { locked: e.target.checked })}
                                />
                                Lock
                              </label>
                              <button
                                className="button ghost"
                                type="button"
                                onClick={() => removeDeviceComponent(idx)}
                                disabled={locked || deviceForm.components.length <= 1}
                              >
                                Remove
                              </button>
                            </div>
                            <div className="inline-row">
                              <select
                                value={selected?.name || ''}
                                onChange={(e) => {
                                  const name = e.target.value
                                  const group = groupsForType.find((g) => g.name === name)
                                  if (!group) {
                                    updateDeviceComponent(idx, { artifactId: '', desiredVersion: '' })
                                  } else {
                                    const pick = group.versions[group.versions.length - 1]
                                    updateDeviceComponent(idx, {
                                      artifactId: pick.artifactId,
                                      desiredVersion: pick.version,
                                      artifactType: normalizeArtifactType(pick.type),
                                    })
                                  }
                                }}
                                disabled={locked}
                              >
                                <option value="">Select artifact</option>
                                {groupsForType.map((group) => (
                                  <option key={group.name} value={group.name}>{group.name}</option>
                                ))}
                              </select>
                              <button
                                className="button ghost"
                                type="button"
                                onClick={() => openArtifactPicker('device', idx)}
                                disabled={locked}
                              >
                                Browse
                              </button>
                              <select
                                value={selected?.version || ''}
                                onChange={(e) => {
                                  const version = e.target.value
                                  const pick = selectedGroup?.versions.find((v) => v.version === version)
                                  if (pick) {
                                    updateDeviceComponent(idx, {
                                      artifactId: pick.artifactId,
                                      desiredVersion: pick.version,
                                      artifactType: normalizeArtifactType(pick.type),
                                    })
                                  }
                                }}
                                disabled={locked}
                              >
                                <option value="">Select version</option>
                                {(selectedGroup?.versions || []).map((artifact) => (
                                  <option key={artifact.artifactId} value={artifact.version}>
                                    {artifact.version}
                                  </option>
                                ))}
                              </select>
                            </div>
                            <div className="inline-row">
                              <input value={row.artifactId} readOnly placeholder="artifact uuid" />
                              <input
                                value={row.desiredVersion}
                                onChange={(e) => updateDeviceComponent(idx, { desiredVersion: e.target.value })}
                                placeholder="desired version"
                                disabled={locked}
                              />
                              <input
                                value={row.desiredConfigRev}
                                onChange={(e) => updateDeviceComponent(idx, { desiredConfigRev: e.target.value })}
                                placeholder="config rev"
                                disabled={locked}
                              />
                            </div>
                          </div>
                        )
                      })}
                      <button className="button ghost" type="button" onClick={addDeviceComponent}>
                        Add component
                      </button>
                    </div>
                    <label>Check-in Interval (sec)</label>
                    <input
                      value={deviceForm.checkinIntervalSec}
                      onChange={(e) => {
                        setDeviceForm({ ...deviceForm, checkinIntervalSec: e.target.value })
                        setDeviceFormDirty(true)
                      }}
                      placeholder="30"
                    />
                    <div className="inline-row">
                      <button className="button" type="submit">Apply</button>
                      {selectedDesired && selectedDesired.source === 'manual' && (
                        <button
                          className="button ghost"
                          type="button"
                          onClick={handleClearDeviceOverride}
                        >
                          Use group desired state
                        </button>
                      )}
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          setDeviceFormDirty(false)
                          setDeviceForm({
                            deviceId: selectedDeviceId,
                            checkinIntervalSec: selectedDesired?.checkinIntervalSec ? String(selectedDesired.checkinIntervalSec) : '',
                            components: buildComponentRows(
                              selectedDesired?.components || {},
                              {
                                artifactId: selectedDesired?.artifactId || '',
                                desiredVersion: selectedDesired?.desiredVersion || deviceDetail.current?.softwareVersion || '',
                                desiredConfigRev: selectedDesired?.desiredConfigRev || deviceDetail.current?.configRev || '',
                              },
                              deviceDetail?.current,
                            ),
                          })
                        }}
                      >
                        Reset
                      </button>
                    </div>
                  </fieldset>
                </form>

                <div className="inline-row">
                  <button className="button ghost" onClick={() => downloadLogs(selectedDeviceId)}>
                    Download Logs
                  </button>
                  {canDecommissionDevices && (
                    <button
                      className="button ghost"
                      onClick={() => handleDeleteDevice(selectedDeviceId)}
                    >
                      Decommission Device
                    </button>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {canManagePendingEnrollments && enrollmentProfileCreateOpen && (
        <div className="modal-backdrop" onClick={() => setEnrollmentProfileCreateOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Create Enrollment Profile</h3>
              <button className="button ghost" onClick={() => setEnrollmentProfileCreateOpen(false)}>
                Close
              </button>
            </div>
            <div className="form compact two-column">
              <div className="field">
                <label>Name</label>
                <input
                  value={enrollmentProfileForm.name}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, name: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Expiration Days</label>
                <input
                  value={enrollmentProfileForm.expiresInDays}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, expiresInDays: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Max Uses (0 = unlimited)</label>
                <input
                  value={enrollmentProfileForm.maxUses}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, maxUses: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Challenge Secret (optional)</label>
                <input
                  value={enrollmentProfileForm.challengeSecret}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, challengeSecret: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Challenge Hint (optional)</label>
                <input
                  value={enrollmentProfileForm.challengeHint}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, challengeHint: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Approval Delay (sec)</label>
                <input
                  type="number"
                  min="0"
                  value={enrollmentProfileForm.approvalDelaySec}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, approvalDelaySec: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Default Labels</label>
                <div className="key-value-list">
                  {enrollmentProfileForm.defaultLabelRows.map((row, idx) => (
                    <div key={`profile-create-label-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...enrollmentProfileForm.defaultLabelRows]
                          next[idx] = { ...row, key: e.target.value }
                          setEnrollmentProfileForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...enrollmentProfileForm.defaultLabelRows]
                          next[idx] = { ...row, value: e.target.value }
                          setEnrollmentProfileForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = enrollmentProfileForm.defaultLabelRows.filter((_, ridx) => ridx !== idx)
                          setEnrollmentProfileForm((prev) => ({ ...prev, defaultLabelRows: next.length > 0 ? next : [{ key: '', value: '' }] }))
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setEnrollmentProfileForm((prev) => ({
                      ...prev,
                      defaultLabelRows: [...prev.defaultLabelRows, { key: '', value: '' }],
                    }))}
                  >
                    Add Label
                  </button>
                </div>
              </div>
              <div className="field">
                <label className="chip">
                  <input
                    type="checkbox"
                    checked={enrollmentProfileForm.allowUnsignedHardwareIdentity}
                    onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, allowUnsignedHardwareIdentity: e.target.checked }))}
                  />
                  Allow unsigned hardware identity
                </label>
              </div>
              <div className="field actions">
                <button className="button ghost" onClick={handleCreateEnrollmentProfile}>
                  Create profile
                </button>
                <button className="button ghost" onClick={() => setEnrollmentProfileCreateOpen(false)}>
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {canManagePendingEnrollments && editingEnrollmentProfileId && (
        <div className="modal-backdrop" onClick={handleCancelEditEnrollmentProfile}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Edit Enrollment Profile</h3>
              <button className="button ghost" onClick={handleCancelEditEnrollmentProfile}>
                Close
              </button>
            </div>
            <div className="form compact two-column">
              <div className="field">
                <label>Name</label>
                <input
                  value={enrollmentProfileEditForm.name}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, name: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Max Uses (0 = unlimited)</label>
                <input
                  value={enrollmentProfileEditForm.maxUses}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, maxUses: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Approval Delay (sec)</label>
                <input
                  type="number"
                  min="0"
                  value={enrollmentProfileEditForm.approvalDelaySec}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, approvalDelaySec: e.target.value }))}
                />
              </div>
              <div className="field">
                <label className="chip">
                  <input
                    type="checkbox"
                    checked={enrollmentProfileEditForm.allowUnsignedHardwareIdentity}
                    onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, allowUnsignedHardwareIdentity: e.target.checked }))}
                  />
                  Allow unsigned hardware identity
                </label>
              </div>
              <div className="field">
                <label className="chip">
                  <input
                    type="checkbox"
                    checked={enrollmentProfileEditForm.challengeEnabled}
                    onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, challengeEnabled: e.target.checked }))}
                  />
                  Challenge enabled
                </label>
              </div>
              <div className="field">
                <label>Challenge Secret (leave blank to keep current)</label>
                <input
                  value={enrollmentProfileEditForm.challengeSecret}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, challengeSecret: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Challenge Hint</label>
                <input
                  value={enrollmentProfileEditForm.challengeHint}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, challengeHint: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Default Labels</label>
                <div className="key-value-list">
                  {enrollmentProfileEditForm.defaultLabelRows.map((row, idx) => (
                    <div key={`profile-edit-label-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...enrollmentProfileEditForm.defaultLabelRows]
                          next[idx] = { ...row, key: e.target.value }
                          setEnrollmentProfileEditForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...enrollmentProfileEditForm.defaultLabelRows]
                          next[idx] = { ...row, value: e.target.value }
                          setEnrollmentProfileEditForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = enrollmentProfileEditForm.defaultLabelRows.filter((_, ridx) => ridx !== idx)
                          setEnrollmentProfileEditForm((prev) => ({ ...prev, defaultLabelRows: next.length > 0 ? next : [{ key: '', value: '' }] }))
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setEnrollmentProfileEditForm((prev) => ({
                      ...prev,
                      defaultLabelRows: [...prev.defaultLabelRows, { key: '', value: '' }],
                    }))}
                  >
                    Add Label
                  </button>
                </div>
              </div>
              <div className="field actions">
                <button className="button ghost" onClick={handleUpdateEnrollmentProfile}>
                  Save profile
                </button>
                <button className="button ghost" onClick={handleCancelEditEnrollmentProfile}>
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {canManageDesiredState && artifactModalOpen && (
        <div
          className="modal-backdrop"
          onClick={() => {
            setArtifactModalOpen(false)
            setArtifactPickerTarget(null)
          }}
        >
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Select Artifact</h3>
              <button
                className="button ghost"
                onClick={() => {
                  setArtifactModalOpen(false)
                  setArtifactPickerTarget(null)
                }}
              >
                Close
              </button>
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>ID</th>
                    <th>Version</th>
                    <th>Signed</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {artifactModalGroups.map((group) => (
                    group.versions.map((a, idx) => (
                      <tr key={a.artifactId} className={idx === 0 ? 'artifact-group-start' : ''}>
                        <td>{idx === 0 ? group.name : ''}</td>
                        <td className="mono">{a.artifactId}</td>
                        <td>{a.version}</td>
                        <td>
                          <span className={`pill ${a.signature ? 'signed' : 'unsigned'}`}>
                            {a.signature ? 'signed' : 'unsigned'}
                          </span>
                        </td>
                        <td>
                          <button
                            className="button ghost"
                          onClick={() => {
                            if (artifactPickerTarget?.scope === 'group') {
                              updateGroupComponent(artifactPickerTarget.index, {
                                artifactId: a.artifactId,
                                desiredVersion: a.version,
                                artifactType: normalizeArtifactType(a.type),
                              })
                            } else if (artifactPickerTarget?.scope === 'groupBulk') {
                              updateGroupMultiDesiredComponent(artifactPickerTarget.index, {
                                artifactId: a.artifactId,
                                desiredVersion: a.version,
                                artifactType: normalizeArtifactType(a.type),
                              })
                            } else if (artifactPickerTarget?.scope === 'device') {
                              updateDeviceComponent(artifactPickerTarget.index, {
                                artifactId: a.artifactId,
                                desiredVersion: a.version,
                                artifactType: normalizeArtifactType(a.type),
                              })
                            }
                            setArtifactModalOpen(false)
                            setArtifactPickerTarget(null)
                          }}
                          >
                            Select
                          </button>
                        </td>
                      </tr>
                    ))
                  ))}
                  {artifactModalGroups.length === 0 && (
                    <tr>
                      <td colSpan={5}>No active artifacts match this component type.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {canManageArtifacts && artifactUploadOpen && (
        <div className="modal-backdrop" onClick={() => setArtifactUploadOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Upload Artifact</h3>
              <button className="button ghost" onClick={() => setArtifactUploadOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleUpload}>
              <div>
                <label>Name</label>
                <input name="name" required placeholder="agent" />
              </div>
              <div>
                <label>Version</label>
                <input name="version" required placeholder="1.0.0" />
              </div>
              <div className="full">
                <label>Bundle</label>
                <input name="file" type="file" required />
              </div>
              <div className="full inline-row">
                <button className="button" type="submit">Upload</button>
                {uploadStatus && <span className="status">{uploadStatus}</span>}
              </div>
            </form>
          </div>
        </div>
      )}

      {canManageGroups && groupModalOpen && (
        <div className="modal-backdrop" onClick={() => setGroupModalOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>{groupForm.groupId ? 'Edit Group' : 'Create Group'}</h3>
              <button className="button ghost" onClick={() => setGroupModalOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleSaveGroup}>
              <div>
                <label>Name</label>
                <input
                  value={groupForm.name}
                  onChange={(e) => setGroupForm({ ...groupForm, name: e.target.value })}
                  placeholder="canary"
                />
              </div>
              <div>
                <label>Region</label>
                <input
                  value={groupForm.region}
                  onChange={(e) => setGroupForm({ ...groupForm, region: e.target.value })}
                  placeholder="west"
                />
              </div>
              <div>
                <label>Role</label>
                <input
                  value={groupForm.role}
                  onChange={(e) => setGroupForm({ ...groupForm, role: e.target.value })}
                  placeholder="edge"
                />
              </div>
              <div>
                <label>Site</label>
                <input
                  value={groupForm.site}
                  onChange={(e) => setGroupForm({ ...groupForm, site: e.target.value })}
                  placeholder="lab-1"
                />
              </div>
              <div className="full">
                <label>Custom Labels</label>
                <div className="key-value-list">
                  {groupForm.custom.map((row, idx) => (
                    <div key={`custom-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...groupForm.custom]
                          next[idx] = { ...row, key: e.target.value }
                          setGroupForm({ ...groupForm, custom: next })
                        }}
                        placeholder="key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...groupForm.custom]
                          next[idx] = { ...row, value: e.target.value }
                          setGroupForm({ ...groupForm, custom: next })
                        }}
                        placeholder="value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = groupForm.custom.filter((_, cidx) => cidx !== idx)
                          setGroupForm({ ...groupForm, custom: next })
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setGroupForm({
                      ...groupForm,
                      custom: [...groupForm.custom, { key: '', value: '' }],
                    })}
                  >
                    Add Custom Label
                  </button>
                </div>
              </div>
              <div className="full inline-row">
                <button className="button" type="submit">
                  {groupForm.groupId ? 'Update Group' : 'Create Group'}
                </button>
                {groupsStatus && <span className="status">{groupsStatus}</span>}
              </div>
            </form>
          </div>
        </div>
      )}

      {canManageGroups && groupMultiEditOpen && (
        <div className="modal-backdrop" onClick={() => setGroupMultiEditOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Edit Selected Groups</h3>
              <button className="button ghost" onClick={() => setGroupMultiEditOpen(false)}>
                Close
              </button>
            </div>
            <div className="detail-note">Selected groups: {selectedGroupIds.length}</div>
            <div className="form">
              <div>
                <label>Region</label>
                <input
                  value={groupMultiEditForm.region}
                  onChange={(e) => setGroupMultiEditForm({ ...groupMultiEditForm, region: e.target.value })}
                  placeholder="leave blank to skip"
                />
              </div>
              <div>
                <label>Role</label>
                <input
                  value={groupMultiEditForm.role}
                  onChange={(e) => setGroupMultiEditForm({ ...groupMultiEditForm, role: e.target.value })}
                  placeholder="leave blank to skip"
                />
              </div>
              <div>
                <label>Site</label>
                <input
                  value={groupMultiEditForm.site}
                  onChange={(e) => setGroupMultiEditForm({ ...groupMultiEditForm, site: e.target.value })}
                  placeholder="leave blank to skip"
                />
              </div>
              <div className="full">
                <label>Custom Selector Values</label>
                <div className="key-value-list">
                  {groupMultiEditForm.custom.map((row, idx) => (
                    <div key={`multi-custom-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...groupMultiEditForm.custom]
                          next[idx] = { ...row, key: e.target.value }
                          setGroupMultiEditForm({ ...groupMultiEditForm, custom: next })
                        }}
                        placeholder="key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...groupMultiEditForm.custom]
                          next[idx] = { ...row, value: e.target.value }
                          setGroupMultiEditForm({ ...groupMultiEditForm, custom: next })
                        }}
                        placeholder="value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = groupMultiEditForm.custom.filter((_, cidx) => cidx !== idx)
                          setGroupMultiEditForm({ ...groupMultiEditForm, custom: next.length > 0 ? next : [{ key: '', value: '' }] })
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setGroupMultiEditForm({
                      ...groupMultiEditForm,
                      custom: [...groupMultiEditForm.custom, { key: '', value: '' }],
                    })}
                  >
                    Add Custom Key
                  </button>
                </div>
              </div>
              <div className="full inline-row">
                <button className="button" type="button" onClick={handleApplyGroupMultiEdit}>
                  Apply to Selected
                </button>
                {groupMultiEditStatus && <span className="status">{groupMultiEditStatus}</span>}
              </div>
            </div>
            {groupMultiEditError && <div className="error">{groupMultiEditError}</div>}
          </div>
        </div>
      )}

      {canManageDesiredState && groupMultiDesiredOpen && (
        <div className="modal-backdrop" onClick={() => setGroupMultiDesiredOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Set Desired for Selected Groups</h3>
              <button className="button ghost" onClick={() => setGroupMultiDesiredOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleApplyGroupMultiDesired}>
              <label>Selected Groups</label>
              <input value={String(selectedGroupIds.length)} readOnly />
              <label>Components</label>
              <div className="component-editor">
                {groupMultiDesiredForm.components.map((row, idx) => {
                  const locked = Boolean(row.locked)
                  const type = normalizeArtifactType(row.artifactType)
                  const groupsForType = filterArtifactGroupsByType(activeArtifactGroups, type)
                  const allGroupsForType = filterArtifactGroupsByType(artifactGroups, type)
                  const selected = artifacts.find((a) => a.artifactId === row.artifactId)
                  const selectedGroup = selected
                    ? (groupsForType.find((g) => g.name === selected.name) ||
                      allGroupsForType.find((g) => g.name === selected.name))
                    : null
                  return (
                    <div className="component-row" key={row.id || `${row.key}-${idx}`}>
                      <div className="inline-row">
                        <input
                          value={row.key}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { key: e.target.value })}
                          placeholder="component key (e.g. app:customer)"
                          disabled={locked}
                        />
                        <input
                          list="artifact-types"
                          value={row.artifactType}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { artifactType: e.target.value })}
                          placeholder="artifact type"
                          disabled={locked}
                        />
                        <select
                          value={row.autoTrackMode || 'inherit'}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { autoTrackMode: e.target.value })}
                          disabled={locked}
                        >
                          {autoTrackModes.map((mode) => (
                            <option key={mode.id} value={mode.id}>{`Auto ${mode.label}`}</option>
                          ))}
                        </select>
                        <label className="inline-toggle">
                          <input
                            type="checkbox"
                            checked={locked}
                            onChange={(e) => updateGroupMultiDesiredComponent(idx, { locked: e.target.checked })}
                          />
                          Lock
                        </label>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => removeGroupMultiDesiredComponent(idx)}
                          disabled={locked || groupMultiDesiredForm.components.length <= 1}
                        >
                          Remove
                        </button>
                      </div>
                      <div className="inline-row">
                        <select
                          value={selected?.name || ''}
                          onChange={(e) => {
                            const name = e.target.value
                            const group = groupsForType.find((g) => g.name === name)
                            if (!group) {
                              updateGroupMultiDesiredComponent(idx, { artifactId: '', desiredVersion: '' })
                            } else {
                              const pick = group.versions[group.versions.length - 1]
                              updateGroupMultiDesiredComponent(idx, {
                                artifactId: pick.artifactId,
                                desiredVersion: pick.version,
                                artifactType: normalizeArtifactType(pick.type),
                              })
                            }
                          }}
                          disabled={locked}
                        >
                          <option value="">Select artifact</option>
                          {groupsForType.map((group) => (
                            <option key={group.name} value={group.name}>{group.name}</option>
                          ))}
                        </select>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => openArtifactPicker('groupBulk', idx)}
                          disabled={locked}
                        >
                          Browse
                        </button>
                        <select
                          value={selected?.version || ''}
                          onChange={(e) => {
                            const version = e.target.value
                            const pick = selectedGroup?.versions.find((v) => v.version === version)
                            if (pick) {
                              updateGroupMultiDesiredComponent(idx, {
                                artifactId: pick.artifactId,
                                desiredVersion: pick.version,
                                artifactType: normalizeArtifactType(pick.type),
                              })
                            }
                          }}
                          disabled={locked}
                        >
                          <option value="">Select version</option>
                          {(selectedGroup?.versions || []).map((artifact) => (
                            <option key={artifact.artifactId} value={artifact.version}>
                              {artifact.version}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className="inline-row">
                        <input value={row.artifactId} readOnly placeholder="artifact uuid" />
                        <input
                          value={row.desiredVersion}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { desiredVersion: e.target.value })}
                          placeholder="desired version"
                          disabled={locked}
                        />
                        <input
                          value={row.desiredConfigRev}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { desiredConfigRev: e.target.value })}
                          placeholder="config rev"
                          disabled={locked}
                        />
                      </div>
                    </div>
                  )
                })}
                <button className="button ghost" type="button" onClick={addGroupMultiDesiredComponent}>
                  Add component
                </button>
              </div>
              <label>Check-in Interval (sec)</label>
              <input
                value={groupMultiDesiredForm.checkinIntervalSec}
                onChange={(e) => setGroupMultiDesiredForm({ ...groupMultiDesiredForm, checkinIntervalSec: e.target.value })}
                placeholder="30"
              />
              <div className="inline-row">
                <button className="button" type="submit">Apply to Selected</button>
                {groupMultiDesiredStatus && <span className="status">{groupMultiDesiredStatus}</span>}
              </div>
            </form>
            {groupMultiDesiredError && <div className="error">{groupMultiDesiredError}</div>}
          </div>
        </div>
      )}

      {canManageGroups && groupBulkOpen && (
        <div className="modal-backdrop" onClick={() => setGroupBulkOpen(false)}>
          <div className="modal bulk-group-modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Bulk Group Management</h3>
              <button className="button ghost" onClick={() => setGroupBulkOpen(false)}>
                Close
              </button>
            </div>
            <div className="bulk-group-toolbar">
              <button className="button ghost" onClick={downloadGroupBulkTemplate}>
                Download template
              </button>
              <label className="button ghost bulk-group-file">
                Load CSV
                <input
                  type="file"
                  accept=".csv,text/csv"
                  onChange={handleGroupBulkFileChange}
                />
              </label>
              <button className="button ghost" onClick={handlePreviewGroupBulk}>
                Preview
              </button>
              <button className="button" onClick={handleApplyGroupBulk} disabled={groupBulkApplying}>
                {groupBulkApplying ? 'Applying…' : 'Apply'}
              </button>
              {groupBulkRollbackCsv && (
                <button className="button ghost" onClick={downloadGroupBulkRollback}>
                  Download rollback CSV
                </button>
              )}
            </div>
            <div className="hint">
              CSV columns: <code>action</code>, <code>groupId</code>, <code>name</code>, <code>region</code>, <code>role</code>,
              <code>site</code>, <code>selector_json</code>, or dynamic <code>selector.&lt;key&gt;</code> columns.
            </div>
            <div className="form">
              <div className="full">
                <label>CSV input</label>
                <textarea
                  className="bulk-group-input"
                  value={groupBulkCsv}
                  onChange={(e) => {
                    setGroupBulkCsv(e.target.value)
                    setGroupBulkPreview(null)
                    setGroupBulkRollbackCsv('')
                    setGroupBulkError('')
                    setGroupBulkStatus('')
                  }}
                  placeholder="action,groupId,name,region,role,site,selector_json"
                />
              </div>
            </div>
            {groupBulkError && <div className="error">{groupBulkError}</div>}
            {groupBulkStatus && <div className="status">{groupBulkStatus}</div>}
            {groupBulkPreview && (
              <>
                <div className="detail-note">
                  Rows: {groupBulkPreview.summary.totalRows} · valid {groupBulkPreview.summary.validRows} · errors {groupBulkPreview.summary.errorRows} ·
                  create {groupBulkPreview.summary.createRows} · update {groupBulkPreview.summary.updateRows} ·
                  delete {groupBulkPreview.summary.deleteRows} · no change {groupBulkPreview.summary.noChangeRows}
                </div>
                <div className="table-wrap bulk-group-table">
                  <table>
                    <thead>
                      <tr>
                        <th>Line</th>
                        <th>Action</th>
                        <th>Group ID</th>
                        <th>Name</th>
                        <th>Selector</th>
                        <th>Preview</th>
                        <th>Apply</th>
                      </tr>
                    </thead>
                    <tbody>
                      {groupBulkPreview.rows.map((row) => (
                        <tr key={row.rowId}>
                          <td>{row.line}</td>
                          <td>{row.action}</td>
                          <td className="mono">{row.groupId || '—'}</td>
                          <td>{row.name || '—'}</td>
                          <td><code>{row.action === 'upsert' ? formatSelector(row.selector) : '—'}</code></td>
                          <td>
                            {row.error ? (
                              <span className="status error-inline">{row.error}</span>
                            ) : (
                              <span className="pill">{row.outcome}</span>
                            )}
                          </td>
                          <td>
                            {row.applyStatus ? (
                              <span className={`pill ${row.applyStatus === 'applied' ? 'signed' : 'unsigned'}`}>
                                {row.applyStatus === 'applied' ? 'ok' : 'failed'}
                              </span>
                            ) : '—'}
                            {row.applyError && <div className="status error-inline">{row.applyError}</div>}
                          </td>
                        </tr>
                      ))}
                      {groupBulkPreview.rows.length === 0 && (
                        <tr>
                          <td colSpan={7}>No rows parsed.</td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {canManageDesiredState && groupDesiredOpen && (
        <div className="modal-backdrop" onClick={() => setGroupDesiredOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Group Desired State</h3>
              <button className="button ghost" onClick={() => setGroupDesiredOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleGroupDesired}>
              <label>Group ID</label>
              <input value={groupDesiredForm.groupId} readOnly />
              <label>Components</label>
              <div className="component-editor">
                {groupDesiredForm.components.map((row, idx) => {
                  const locked = Boolean(row.locked)
                  const type = normalizeArtifactType(row.artifactType)
                  const groupsForType = filterArtifactGroupsByType(activeArtifactGroups, type)
                  const allGroupsForType = filterArtifactGroupsByType(artifactGroups, type)
                  const selected = artifacts.find((a) => a.artifactId === row.artifactId)
                  const selectedGroup = selected
                    ? (groupsForType.find((g) => g.name === selected.name) ||
                      allGroupsForType.find((g) => g.name === selected.name))
                    : null
                  return (
                    <div className="component-row" key={row.id || `${row.key}-${idx}`}>
                      <div className="inline-row">
                        <input
                          value={row.key}
                          onChange={(e) => updateGroupComponent(idx, { key: e.target.value })}
                          placeholder="component key (e.g. app:customer)"
                          disabled={locked}
                        />
                        <input
                          list="artifact-types"
                          value={row.artifactType}
                          onChange={(e) => updateGroupComponent(idx, { artifactType: e.target.value })}
                          placeholder="artifact type"
                          disabled={locked}
                        />
                        <select
                          value={row.autoTrackMode || 'inherit'}
                          onChange={(e) => updateGroupComponent(idx, { autoTrackMode: e.target.value })}
                          disabled={locked}
                        >
                          {autoTrackModes.map((mode) => (
                            <option key={mode.id} value={mode.id}>{`Auto ${mode.label}`}</option>
                          ))}
                        </select>
                        <label className="inline-toggle">
                          <input
                            type="checkbox"
                            checked={locked}
                            onChange={(e) => updateGroupComponent(idx, { locked: e.target.checked })}
                          />
                          Lock
                        </label>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => removeGroupComponent(idx)}
                          disabled={locked || groupDesiredForm.components.length <= 1}
                        >
                          Remove
                        </button>
                      </div>
                      <div className="inline-row">
                        <select
                          value={selected?.name || ''}
                          onChange={(e) => {
                            const name = e.target.value
                            const group = groupsForType.find((g) => g.name === name)
                            if (!group) {
                              updateGroupComponent(idx, { artifactId: '', desiredVersion: '' })
                            } else {
                              const pick = group.versions[group.versions.length - 1]
                              updateGroupComponent(idx, {
                                artifactId: pick.artifactId,
                                desiredVersion: pick.version,
                                artifactType: normalizeArtifactType(pick.type),
                              })
                            }
                          }}
                          disabled={locked}
                        >
                          <option value="">Select artifact</option>
                          {groupsForType.map((group) => (
                            <option key={group.name} value={group.name}>{group.name}</option>
                          ))}
                        </select>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => openArtifactPicker('group', idx)}
                          disabled={locked}
                        >
                          Browse
                        </button>
                        <select
                          value={selected?.version || ''}
                          onChange={(e) => {
                            const version = e.target.value
                            const pick = selectedGroup?.versions.find((v) => v.version === version)
                            if (pick) {
                              updateGroupComponent(idx, {
                                artifactId: pick.artifactId,
                                desiredVersion: pick.version,
                                artifactType: normalizeArtifactType(pick.type),
                              })
                            }
                          }}
                          disabled={locked}
                        >
                          <option value="">Select version</option>
                          {(selectedGroup?.versions || []).map((artifact) => (
                            <option key={artifact.artifactId} value={artifact.version}>
                              {artifact.version}
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className="inline-row">
                        <input value={row.artifactId} readOnly placeholder="artifact uuid" />
                        <input
                          value={row.desiredVersion}
                          onChange={(e) => updateGroupComponent(idx, { desiredVersion: e.target.value })}
                          placeholder="desired version"
                          disabled={locked}
                        />
                        <input
                          value={row.desiredConfigRev}
                          onChange={(e) => updateGroupComponent(idx, { desiredConfigRev: e.target.value })}
                          placeholder="config rev"
                          disabled={locked}
                        />
                      </div>
                    </div>
                  )
                })}
                <button className="button ghost" type="button" onClick={addGroupComponent}>
                  Add component
                </button>
              </div>
              <label>Check-in Interval (sec)</label>
              <input
                value={groupDesiredForm.checkinIntervalSec}
                onChange={(e) => setGroupDesiredForm({ ...groupDesiredForm, checkinIntervalSec: e.target.value })}
                placeholder="30"
              />
              <div className="inline-row">
                <button className="button" type="submit">Apply</button>
                {selectedGroupDesired && (
                  <button className="button ghost" type="button" onClick={handleClearGroupDesired}>
                    Clear group desired state
                  </button>
                )}
                {groupsStatus && <span className="status">{groupsStatus}</span>}
              </div>
            </form>
          </div>
        </div>
      )}
      <datalist id="artifact-types">
        {componentTypes.map((comp) => (
          <option key={comp.id} value={comp.id}>{comp.label}</option>
        ))}
      </datalist>
    </div>
  )
}
