export const nav = [
  { id: 'dashboard', label: 'Dashboard', icon: 'icon-dashboard' },
  { id: 'metrics', label: 'Metrics', icon: 'icon-metrics' },
  { id: 'logs', label: 'Logs', icon: 'icon-logs' },
  { id: 'security', label: 'Security', icon: 'icon-security' },
  { id: 'settings', label: 'Settings', icon: 'icon-settings' },
  { id: 'global', label: 'Global Plane', icon: 'icon-global' },
]

export const logsNav = [
  { id: 'events', label: 'Live Events' },
  { id: 'device', label: 'Device Logs' },
  { id: 'audit', label: 'Audit Log' },
]

export const componentTypes = [
  { id: 'app_bundle', label: 'App Bundle' },
  { id: 'config_bundle', label: 'Config Bundle' },
  { id: 'data_bundle', label: 'Data Bundle' },
  { id: 'firmware', label: 'Firmware' },
  { id: 'container_image', label: 'Container Image' },
  { id: 'agent_bundle', label: 'Agent Bundle' },
]

export const rangeOptions = [
  { id: '15m', label: '15m', ms: 15 * 60 * 1000 },
  { id: '1h', label: '1h', ms: 60 * 60 * 1000 },
  { id: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
  { id: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
]

export const rangeMsById = rangeOptions.reduce((acc, item) => {
  acc[item.id] = item.ms
  return acc
}, {})

export const chartColors = {
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

export const autoTrackModes = [
  { id: 'inherit', label: 'Inherit' },
  { id: 'enabled', label: 'Auto-follow latest' },
  { id: 'disabled', label: 'Disabled' },
]

export const groupTrackingModes = [
  { id: 'inherit', label: 'Inherit global default' },
  { id: 'enabled', label: 'Auto-follow latest' },
  { id: 'disabled', label: 'Disabled' },
]

export const deviceTrackingModes = [
  { id: 'inherit', label: 'Inherit from group' },
  { id: 'enabled', label: 'Auto-follow latest' },
  { id: 'disabled', label: 'Disabled' },
]

export const trustOverrideModes = [
  { id: 'inherit', label: 'Inherit' },
  { id: 'allow_unsigned', label: 'Allow unsigned' },
  { id: 'warn_unsigned', label: 'Warn unsigned' },
  { id: 'require_verified', label: 'Require verified' },
]

export const signatureTypeOptions = [
  { id: 'ed25519', label: 'Ed25519' },
  { id: 'cosign', label: 'Cosign (key-based)' },
]

export function parsePolicyObject(raw) {
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

export function getAutoTrackModeFromPolicy(raw) {
  const policy = parsePolicyObject(raw)
  const mode = policy?.hwops?.tracking?.mode || policy?.hwops?.autoVersion?.mode
  if (mode === 'enabled' || mode === 'disabled') return mode
  return 'inherit'
}

export function getTrackingPolicyFromPolicy(raw) {
  const policy = parsePolicyObject(raw)
  const hwops = policy.hwops && typeof policy.hwops === 'object' ? { ...policy.hwops } : {}
  const tracking = hwops.tracking && typeof hwops.tracking === 'object' ? { ...hwops.tracking } : {}
  const mode = tracking.mode || hwops?.autoVersion?.mode
  return {
    mode: mode === 'enabled' || mode === 'disabled' ? mode : 'inherit',
    name: String(tracking.name || '').trim(),
    artifactType: normalizeArtifactType(tracking.artifactType || ''),
  }
}

export function mergeAutoTrackModeIntoPolicy(raw, mode) {
  return mergeTrackingIntoPolicy(raw, { mode })
}

export function mergeTrackingIntoPolicy(raw, { mode = 'inherit', name = '', artifactType = '' } = {}) {
  const policy = parsePolicyObject(raw)
  const hwops = policy.hwops && typeof policy.hwops === 'object' ? { ...policy.hwops } : {}
  const normalizedMode = mode === 'enabled' || mode === 'disabled' ? mode : 'inherit'
  const normalizedName = String(name || '').trim()
  const normalizedType = normalizeArtifactType(artifactType)

  if (normalizedMode === 'enabled' || normalizedMode === 'inherit') {
    const tracking = {
      mode: normalizedMode,
      name: normalizedName,
      artifactType: normalizedType,
    }
    hwops.tracking = tracking
  } else if (hwops.tracking) {
    hwops.tracking = { mode: 'disabled' }
  }

  if (normalizedMode === 'enabled' || normalizedMode === 'disabled') {
    hwops.autoVersion = { mode: normalizedMode }
  } else if (hwops.autoVersion) {
    delete hwops.autoVersion
  }

  if (normalizedMode === 'disabled' && !hwops.tracking) {
    hwops.tracking = { mode: 'disabled' }
  }

  if (hwops.tracking) {
    if (!hwops.tracking.name) delete hwops.tracking.name
    if (!hwops.tracking.artifactType) delete hwops.tracking.artifactType
    if (Object.keys(hwops.tracking).length === 0) delete hwops.tracking
  }

  if (Object.keys(hwops).length > 0) {
    policy.hwops = hwops
  } else {
    delete policy.hwops
  }
  return policy
}

export function verificationModeLabel(mode) {
  switch (String(mode || '').trim().toLowerCase()) {
    case 'allow_unsigned':
      return 'Allow unsigned'
    case 'require_verified':
      return 'Require verified'
    case 'warn_unsigned':
    default:
      return 'Warn unsigned'
  }
}

export function trustPolicyStrictness(mode) {
  switch (String(mode || '').trim().toLowerCase()) {
    case 'allow_unsigned':
      return 0
    case 'warn_unsigned':
      return 1
    case 'require_verified':
      return 2
    default:
      return 1
  }
}

export function normalizeTrustOverride(raw) {
  const policy = parsePolicyObject(raw)
  const allowedSigningKeyIds = Array.isArray(policy.allowedSigningKeyIds)
    ? policy.allowedSigningKeyIds.filter(Boolean)
    : (policy.signingKeyId ? [String(policy.signingKeyId)] : [])
  const allowedSignatureTypes = Array.isArray(policy.allowedSignatureTypes)
    ? policy.allowedSignatureTypes.filter(Boolean)
    : []
  let verificationMode = String(policy.verificationMode || '').trim().toLowerCase()
  if (!verificationMode && policy.requireSignature) verificationMode = 'require_verified'
  return {
    verificationMode: verificationMode || 'inherit',
    allowedSigningKeyIds,
    allowedSignatureTypes,
  }
}

export function mergeTrustOverrideIntoPolicy(raw, override) {
  const policy = parsePolicyObject(raw)
  const nextMode = String(override?.verificationMode || 'inherit').trim().toLowerCase()
  const allowedSigningKeyIds = Array.isArray(override?.allowedSigningKeyIds)
    ? override.allowedSigningKeyIds.filter(Boolean)
    : []
  const allowedSignatureTypes = Array.isArray(override?.allowedSignatureTypes)
    ? override.allowedSignatureTypes.filter(Boolean)
    : []

  if (nextMode && nextMode !== 'inherit') {
    policy.verificationMode = nextMode
    if (nextMode === 'require_verified') {
      policy.requireSignature = true
    } else {
      delete policy.requireSignature
    }
  } else {
    delete policy.verificationMode
    delete policy.requireSignature
  }

  if (allowedSigningKeyIds.length > 0) {
    policy.allowedSigningKeyIds = [...allowedSigningKeyIds]
    if (allowedSigningKeyIds.length === 1) {
      policy.signingKeyId = allowedSigningKeyIds[0]
    } else {
      delete policy.signingKeyId
    }
  } else {
    delete policy.allowedSigningKeyIds
    delete policy.signingKeyId
  }

  if (allowedSignatureTypes.length > 0) {
    policy.allowedSignatureTypes = [...allowedSignatureTypes]
  } else {
    delete policy.allowedSignatureTypes
  }

  return policy
}

export function resolveEffectiveTrustPolicy(raw, globalPolicy) {
  const base = {
    verificationMode: String(globalPolicy?.verificationMode || 'warn_unsigned'),
    allowedSigningKeyIds: Array.isArray(globalPolicy?.allowedSigningKeyIds) ? globalPolicy.allowedSigningKeyIds.filter(Boolean) : [],
    allowedSignatureTypes: Array.isArray(globalPolicy?.allowedSignatureTypes) ? globalPolicy.allowedSignatureTypes.filter(Boolean) : [],
  }
  const override = normalizeTrustOverride(raw)
  if (
    override.verificationMode &&
    override.verificationMode !== 'inherit' &&
    trustPolicyStrictness(override.verificationMode) >= trustPolicyStrictness(base.verificationMode)
  ) {
    base.verificationMode = override.verificationMode
  }
  if (override.allowedSigningKeyIds.length > 0) {
    base.allowedSigningKeyIds = override.allowedSigningKeyIds
  }
  if (override.allowedSignatureTypes.length > 0) {
    base.allowedSignatureTypes = override.allowedSignatureTypes
  }
  return base
}

export function hasTrustOverride(raw) {
  const override = normalizeTrustOverride(raw)
  return (
    override.verificationMode !== 'inherit' ||
    override.allowedSigningKeyIds.length > 0 ||
    override.allowedSignatureTypes.length > 0
  )
}

export function trustPolicySummary(raw, globalPolicy) {
  const effective = resolveEffectiveTrustPolicy(raw, globalPolicy)
  const parts = [hasTrustOverride(raw) ? 'override' : 'inherited', verificationModeLabel(effective.verificationMode)]
  if (effective.allowedSignatureTypes.length > 0) {
    parts.push(`types ${effective.allowedSignatureTypes.join(', ')}`)
  }
  if (effective.allowedSigningKeyIds.length > 0) {
    parts.push(`keys ${effective.allowedSigningKeyIds.length}`)
  }
  return parts.join(' · ')
}

export function formatChartValue(value, integerOnly) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  if (integerOnly) return Math.round(value).toLocaleString()
  return Number(value).toLocaleString()
}

export function TimeSeriesChart({ series = [], rangeMs = rangeMsById['1h'], height = 180, integerOnly = false }) {
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

export function ChartLegend({ series }) {
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

export function newComponentRow(overrides = {}) {
  return {
    id: typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : `cmp-${Date.now()}-${Math.random()}`,
    key: '',
    artifactType: '',
    artifactId: '',
    desiredVersion: '',
    desiredConfigRev: '',
    policy: {},
    autoTrackMode: 'inherit',
    trackingName: '',
    locked: false,
    ...overrides,
  }
}

export function parseSemver4(value) {
  const raw = String(value || '').trim().replace(/^v/i, '')
  if (!raw) return null
  const parts = raw.split('.')
  if (parts.length !== 3 && parts.length !== 4) return null
  const values = parts.map((part) => Number.parseInt(part, 10))
  if (values.some((part) => Number.isNaN(part) || part < 0)) return null
  while (values.length < 4) values.push(0)
  return values
}

export function compareSemver4(a, b) {
  const left = Array.isArray(a) ? a : parseSemver4(a)
  const right = Array.isArray(b) ? b : parseSemver4(b)
  if (!left || !right) return 0
  for (let idx = 0; idx < 4; idx += 1) {
    if (left[idx] > right[idx]) return 1
    if (left[idx] < right[idx]) return -1
  }
  return 0
}

export function normalizeArtifactType(val) {
  if (!val) return ''
  return String(val).trim().toLowerCase()
}

export function normalizeArtifactStatus(val) {
  const normalized = String(val || '').trim().toLowerCase()
  if (!normalized) return 'active'
  return normalized
}

export function normalizeVerificationStatus(val, artifact) {
  const normalized = String(val || '').trim().toLowerCase()
  if (normalized) return normalized
  if (artifact?.signature) return 'legacy'
  return 'unsigned'
}

export function verificationPillLabel(artifact) {
  const status = normalizeVerificationStatus(artifact?.verificationStatus, artifact)
  switch (status) {
    case 'verified':
      return 'verified'
    case 'legacy':
      return 'legacy'
    case 'failed':
      return 'failed'
    case 'untrusted':
      return 'untrusted'
    default:
      return 'unsigned'
  }
}

export function artifactSignerSummary(artifact) {
  const parts = []
  if (artifact?.signatureType) parts.push(String(artifact.signatureType))
  if (artifact?.signatureKeyId) parts.push(String(artifact.signatureKeyId))
  return parts.join(' · ') || '—'
}

export function artifactAllowedByTrustPolicy(artifact, policy) {
  const mode = String(policy?.verificationMode || 'warn_unsigned')
  if (mode !== 'require_verified') return true
  const status = normalizeVerificationStatus(artifact?.verificationStatus, artifact)
  if (status !== 'verified') return false
  const allowedKeyIds = Array.isArray(policy?.allowedSigningKeyIds) ? policy.allowedSigningKeyIds.filter(Boolean) : []
  if (allowedKeyIds.length > 0 && !allowedKeyIds.includes(String(artifact?.signatureKeyId || ''))) {
    return false
  }
  const allowedSignatureTypes = Array.isArray(policy?.allowedSignatureTypes)
    ? policy.allowedSignatureTypes.filter(Boolean)
    : []
  if (allowedSignatureTypes.length > 0 && !allowedSignatureTypes.includes(String(artifact?.signatureType || ''))) {
    return false
  }
  return true
}

export function filterArtifactGroupsByType(groups, type) {
  const target = normalizeArtifactType(type)
  if (!target) return groups
  return groups
    .map((group) => ({
      ...group,
      versions: group.versions.filter((artifact) => normalizeArtifactType(artifact.type) === target),
    }))
    .filter((group) => group.versions.length > 0)
}

export function filterArtifactGroupsByTrustPolicy(groups, policy) {
  return (groups || [])
    .map((group) => ({
      ...group,
      versions: (group.versions || []).filter((artifact) => artifactAllowedByTrustPolicy(artifact, policy)),
    }))
    .filter((group) => group.versions.length > 0)
}

export function fallbackComponentType(key, artifactType) {
  const normalized = normalizeArtifactType(artifactType)
  if (normalized) return normalized
  if (key === 'agent_bundle') return 'agent_bundle'
  if (key === 'app_bundle') return 'app_bundle'
  return ''
}

export function parseCSVLine(line) {
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

export function parseCSV(text) {
  const trimmed = text.trim()
  if (!trimmed) return { header: [], rows: [] }
  const lines = trimmed.split(/\r?\n/)
  const header = parseCSVLine(lines[0])
  const rows = lines.slice(1).map(parseCSVLine)
  return { header, rows }
}

export function normalizeObject(value) {
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

export function selectorMatches(labels, selector) {
  if (!selector || Object.keys(selector).length === 0) return true
  if (!labels) return false
  return Object.entries(selector).every(([key, val]) => labels[key] === val)
}

export function selectorToForm(selector) {
  const region = selector.region ?? ''
  const role = selector.role ?? ''
  const site = selector.site ?? ''
  const custom = Object.entries(selector)
    .filter(([key]) => !['region', 'role', 'site'].includes(key))
    .map(([key, value]) => ({ key, value: String(value ?? '') }))
  return { region, role, site, custom }
}

export function objectToKeyValueRows(obj) {
  const normalized = normalizeObject(obj)
  const entries = Object.entries(normalized)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => ({ key: String(key), value: String(value ?? '') }))
  return entries.length > 0 ? entries : [{ key: '', value: '' }]
}

export function keyValueRowsToObject(rows) {
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

export function formatSelector(selector) {
  const entries = Object.entries(selector || {})
  if (entries.length === 0) return '—'
  return entries.map(([key, value]) => `${key}=${value}`).join(', ')
}

export function normalizeSelector(selector) {
  const normalized = {}
  const obj = normalizeObject(selector)
  Object.entries(obj).forEach(([key, value]) => {
    const k = String(key || '').trim()
    if (!k) return
    normalized[k] = String(value ?? '')
  })
  return normalized
}

export function stableSelectorString(selector) {
  const sorted = Object.entries(selector || {}).sort(([a], [b]) => a.localeCompare(b))
  return JSON.stringify(Object.fromEntries(sorted))
}

export function isUUID(value) {
  const input = String(value || '').trim()
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(input)
}

export function csvEscape(value) {
  const input = String(value ?? '')
  if (!/[",\n]/.test(input)) return input
  return `"${input.replace(/"/g, '""')}"`
}

export function buildBulkGroupsRollbackCsv(rows) {
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

export function parseBulkGroupsCsv(csvText, existingGroups) {
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

export function toIsoIfValid(value) {
  if (!value) return ''
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return ''
  return dt.toISOString()
}

export function formatTime(value) {
  if (!value) return '—'
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return String(value)
  return dt.toLocaleString()
}

export function formatNumber(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return Number(value).toLocaleString()
}

export function formatBytes(value) {
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

export function formatDurationSeconds(seconds) {
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

export function downloadTextFile(filename, text) {
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

export function formatObjectSummary(value) {
  const obj = normalizeObject(value)
  const entries = Object.entries(obj)
  if (entries.length === 0) return '—'
  return entries.map(([key, val]) => `${key}=${String(val)}`).join(', ')
}

export function parsePrometheusMetrics(text) {
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

export function sumLabeled(metrics, name) {
  const items = metrics.labeled?.[name] || []
  return items.reduce((acc, item) => acc + Number(item.value || 0), 0)
}

export function groupLabeledMetrics(metrics, name, labelKey, mapLabel) {
  const items = metrics.labeled?.[name] || []
  const out = {}
  items.forEach((item) => {
    const raw = item.labels?.[labelKey] || 'unknown'
    const key = mapLabel ? mapLabel(raw) : raw
    out[key] = (out[key] || 0) + Number(item.value || 0)
  })
  return out
}
