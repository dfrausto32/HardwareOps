import { useCallback, useEffect, useRef, useState } from 'react'

const STATUS_COLORS = {
  active: '#3bd487',
  stale: '#9aa3ad',
  offline: '#f27272',
  degraded: '#f1c76f',
}

function statusDot(status) {
  return (
    <span
      style={{
        display: 'inline-block',
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: STATUS_COLORS[status] || '#9aa3ad',
        marginRight: 6,
        flexShrink: 0,
      }}
      aria-hidden="true"
    />
  )
}

function RelativeTime({ iso }) {
  if (!iso) return <span className="muted">—</span>
  const d = new Date(iso)
  const diffMs = Date.now() - d.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  if (diffMins < 2) return <span>just now</span>
  if (diffMins < 60) return <span>{diffMins}m ago</span>
  const diffH = Math.floor(diffMins / 60)
  if (diffH < 24) return <span>{diffH}h ago</span>
  return <span>{Math.floor(diffH / 24)}d ago</span>
}

function HealthCard({ plane, health }) {
  const syncError = plane.lastSyncError
  const synced = plane.lastSyncAt

  return (
    <div className="card" style={{ padding: '16px 20px', minWidth: 220 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <div style={{ fontWeight: 600, fontSize: 14 }}>{plane.name}</div>
        <span
          className={`chip ${syncError ? 'error' : 'success'}`}
          style={{ fontSize: 11 }}
        >
          {syncError ? 'sync error' : 'synced'}
        </span>
      </div>
      <div className="muted" style={{ fontSize: 11, marginBottom: 10 }}>
        {plane.baseUrl} · last sync <RelativeTime iso={synced} />
      </div>
      {syncError && (
        <div
          style={{
            fontSize: 11,
            color: '#f27272',
            marginBottom: 10,
            wordBreak: 'break-all',
            background: 'rgba(242,114,114,0.08)',
            borderRadius: 4,
            padding: '4px 8px',
          }}
        >
          {syncError}
        </div>
      )}
      {health ? (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px 12px' }}>
          {[
            ['Total', health.totalDevices, '#8aa5ff'],
            ['Active', health.activeDevices, '#3bd487'],
            ['Stale', health.staleDevices, '#9aa3ad'],
            ['Offline', health.offlineDevices, '#f27272'],
            ['Degraded', health.degradedDevices, '#f1c76f'],
          ].map(([label, count, color]) => (
            <div key={label} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ fontSize: 15, fontWeight: 700, color }}>{count ?? 0}</span>
              <span className="muted" style={{ fontSize: 11 }}>{label}</span>
            </div>
          ))}
        </div>
      ) : (
        <div className="muted" style={{ fontSize: 12 }}>No health data yet</div>
      )}
    </div>
  )
}

export default function GlobalPage({
  view,
  globalPlaneUrl,
  onChangeGlobalPlaneUrl,
  formatTime,
}) {
  const [urlInput, setUrlInput] = useState(globalPlaneUrl || '')
  const [token, setToken] = useState(() => {
    try { return window.localStorage.getItem('hwops_global_token') || '' } catch { return '' }
  })
  const [planes, setPlanes] = useState([])
  const [healthMap, setHealthMap] = useState({}) // planeId → health snapshot
  const [devices, setDevices] = useState([])
  const [artifacts, setArtifacts] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [deviceFilter, setDeviceFilter] = useState('')
  const [artifactFilter, setArtifactFilter] = useState('')
  const [tab, setTab] = useState('health') // health | devices | artifacts
  const [registerOpen, setRegisterOpen] = useState(false)
  const [registerForm, setRegisterForm] = useState({ name: '', baseUrl: '', serviceToken: '', tlsCaPem: '', syncIntervalSeconds: '60' })
  const [registerError, setRegisterError] = useState(null)
  const [registerSaving, setRegisterSaving] = useState(false)
  const pollRef = useRef(null)

  const effectiveUrl = globalPlaneUrl || urlInput

  const apiFetch = useCallback(async (path, options = {}) => {
    if (!effectiveUrl) throw new Error('Global-plane URL not set')
    const headers = { 'Content-Type': 'application/json' }
    if (token) headers['Authorization'] = `Bearer ${token}`
    const resp = await fetch(`${effectiveUrl}${path}`, { ...options, headers: { ...headers, ...(options.headers || {}) } })
    if (!resp.ok) {
      const text = await resp.text()
      throw new Error(`${resp.status}: ${text.slice(0, 200)}`)
    }
    if (resp.status === 204) return null
    return resp.json()
  }, [effectiveUrl, token])

  const loadAll = useCallback(async () => {
    if (!effectiveUrl) return
    setLoading(true)
    setError(null)
    try {
      const [planesData, healthData, devicesData, artifactsData] = await Promise.all([
        apiFetch('/api/v1/planes'),
        apiFetch('/api/v1/health/summary'),
        apiFetch('/api/v1/devices'),
        apiFetch('/api/v1/artifacts'),
      ])
      setPlanes(planesData || [])
      const hm = {}
      if (healthData?.planes) {
        for (const ph of healthData.planes) hm[ph.planeId] = ph
      }
      setHealthMap(hm)
      setDevices(devicesData || [])
      setArtifacts(artifactsData || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [apiFetch, effectiveUrl])

  // Auto-refresh every 30s while view is active
  useEffect(() => {
    if (view !== 'global') return
    loadAll()
    pollRef.current = setInterval(loadAll, 30000)
    return () => clearInterval(pollRef.current)
  }, [view, loadAll])

  const handleConnect = () => {
    try { window.localStorage.setItem('hwops_global_token', token) } catch {}
    if (onChangeGlobalPlaneUrl) onChangeGlobalPlaneUrl(urlInput)
  }

  const handleRegister = async (e) => {
    e.preventDefault()
    setRegisterError(null)
    setRegisterSaving(true)
    try {
      await apiFetch('/api/v1/planes', {
        method: 'POST',
        body: JSON.stringify({
          name: registerForm.name,
          baseUrl: registerForm.baseUrl,
          serviceToken: registerForm.serviceToken,
          tlsCaPem: registerForm.tlsCaPem || undefined,
          syncIntervalSeconds: parseInt(registerForm.syncIntervalSeconds, 10) || 60,
        }),
      })
      setRegisterOpen(false)
      setRegisterForm({ name: '', baseUrl: '', serviceToken: '', tlsCaPem: '', syncIntervalSeconds: '60' })
      loadAll()
    } catch (err) {
      setRegisterError(err.message)
    } finally {
      setRegisterSaving(false)
    }
  }

  const filteredDevices = devices.filter(d => {
    if (!deviceFilter) return true
    const q = deviceFilter.toLowerCase()
    return d.deviceId?.toLowerCase().includes(q) || d.status?.toLowerCase().includes(q)
  })

  const filteredArtifacts = artifacts.filter(a => {
    if (!artifactFilter) return true
    const q = artifactFilter.toLowerCase()
    return a.name?.toLowerCase().includes(q) || a.version?.toLowerCase().includes(q) || a.artifactType?.toLowerCase().includes(q)
  })

  if (view !== 'global') return null

  return (
    <section id="global" className="card settings-card">
      <div className="section-header settings-header">
        <h2>Global Plane</h2>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="button ghost" onClick={loadAll} disabled={loading}>
            {loading ? 'Loading…' : 'Refresh'}
          </button>
          {effectiveUrl && (
            <button className="button" onClick={() => setRegisterOpen(true)}>
              Register plane
            </button>
          )}
        </div>
      </div>

      {/* Connection config */}
      <div className="card" style={{ padding: '14px 18px', marginBottom: 20 }}>
        <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 10 }}>Global-plane connection</div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end' }}>
          <div style={{ flex: '1 1 260px' }}>
            <label className="label">URL</label>
            <input
              className="input"
              placeholder="https://global-plane-host:8090"
              value={urlInput}
              onChange={e => setUrlInput(e.target.value)}
            />
          </div>
          <div style={{ flex: '1 1 260px' }}>
            <label className="label">Bearer token (optional)</label>
            <input
              className="input"
              type="password"
              placeholder="JWT or service token"
              value={token}
              onChange={e => setToken(e.target.value)}
            />
          </div>
          <button className="button" onClick={handleConnect}>Connect</button>
        </div>
        {error && <div style={{ color: '#f27272', marginTop: 8, fontSize: 12 }}>{error}</div>}
      </div>

      {!effectiveUrl && (
        <div className="muted" style={{ textAlign: 'center', padding: '40px 0' }}>
          Enter the global-plane URL above and click Connect.
        </div>
      )}

      {effectiveUrl && (
        <>
          {/* Tab bar */}
          <div style={{ display: 'flex', gap: 4, marginBottom: 16, borderBottom: '1px solid var(--border)', paddingBottom: 0 }}>
            {[['health', `Planes (${planes.length})`], ['devices', `Devices (${devices.length})`], ['artifacts', `Artifacts (${artifacts.length})`]].map(([id, label]) => (
              <button
                key={id}
                className={`chip ${tab === id ? 'active' : ''}`}
                style={{ borderRadius: '4px 4px 0 0', marginBottom: -1 }}
                onClick={() => setTab(id)}
                type="button"
              >
                {label}
              </button>
            ))}
          </div>

          {/* Health / planes tab */}
          {tab === 'health' && (
            <>
              {planes.length === 0 ? (
                <div className="muted" style={{ textAlign: 'center', padding: '32px 0' }}>
                  No regional planes registered. Click <strong>Register plane</strong> to add one.
                </div>
              ) : (
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16 }}>
                  {planes.map(p => (
                    <HealthCard key={p.planeId} plane={p} health={healthMap[p.planeId]} />
                  ))}
                </div>
              )}
            </>
          )}

          {/* Devices tab */}
          {tab === 'devices' && (
            <>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                <input
                  className="input"
                  placeholder="Filter by device ID or status…"
                  value={deviceFilter}
                  onChange={e => setDeviceFilter(e.target.value)}
                  style={{ maxWidth: 320 }}
                />
              </div>
              <table className="table">
                <thead>
                  <tr>
                    <th>Device ID</th>
                    <th>Status</th>
                    <th>Region</th>
                    <th>Last seen</th>
                    <th>Synced</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredDevices.length === 0 ? (
                    <tr><td colSpan={5} style={{ textAlign: 'center', padding: '24px 0' }} className="muted">No devices found</td></tr>
                  ) : filteredDevices.map(d => {
                    const plane = planes.find(p => p.planeId === d.planeId)
                    return (
                      <tr key={`${d.planeId}:${d.deviceId}`}>
                        <td><code style={{ fontSize: 12 }}>{d.deviceId}</code></td>
                        <td style={{ display: 'flex', alignItems: 'center', paddingTop: 10 }}>
                          {statusDot(d.status)}{d.status || '—'}
                        </td>
                        <td className="muted" style={{ fontSize: 12 }}>{plane?.name || d.planeId}</td>
                        <td className="muted" style={{ fontSize: 12 }}><RelativeTime iso={d.lastSeen} /></td>
                        <td className="muted" style={{ fontSize: 12 }}><RelativeTime iso={d.syncedAt} /></td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </>
          )}

          {/* Artifacts tab */}
          {tab === 'artifacts' && (
            <>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                <input
                  className="input"
                  placeholder="Filter by name, version or type…"
                  value={artifactFilter}
                  onChange={e => setArtifactFilter(e.target.value)}
                  style={{ maxWidth: 320 }}
                />
              </div>
              <table className="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Version</th>
                    <th>Type</th>
                    <th>Status</th>
                    <th>Region</th>
                    <th>Size</th>
                    <th>Synced</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredArtifacts.length === 0 ? (
                    <tr><td colSpan={7} style={{ textAlign: 'center', padding: '24px 0' }} className="muted">No artifacts found</td></tr>
                  ) : filteredArtifacts.map(a => {
                    const plane = planes.find(p => p.planeId === a.planeId)
                    const sizeKb = a.sizeBytes ? `${Math.round(a.sizeBytes / 1024)} KB` : '—'
                    return (
                      <tr key={`${a.planeId}:${a.artifactId}`}>
                        <td style={{ fontWeight: 500 }}>{a.name || '—'}</td>
                        <td><code style={{ fontSize: 12 }}>{a.version || '—'}</code></td>
                        <td className="muted" style={{ fontSize: 12 }}>{a.artifactType || '—'}</td>
                        <td>
                          <span className={`chip ${a.status === 'active' ? 'success' : a.status === 'deprecated' ? 'warning' : ''}`} style={{ fontSize: 11 }}>
                            {a.status || '—'}
                          </span>
                        </td>
                        <td className="muted" style={{ fontSize: 12 }}>{plane?.name || a.planeId}</td>
                        <td className="muted" style={{ fontSize: 12 }}>{sizeKb}</td>
                        <td className="muted" style={{ fontSize: 12 }}><RelativeTime iso={a.syncedAt} /></td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </>
          )}
        </>
      )}

      {/* Register plane modal */}
      {registerOpen && (
        <div className="modal-overlay" onClick={() => setRegisterOpen(false)}>
          <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 480 }}>
            <div className="modal-header">
              <h3>Register regional plane</h3>
              <button className="modal-close" onClick={() => setRegisterOpen(false)}>×</button>
            </div>
            <form onSubmit={handleRegister}>
              <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div>
                  <label className="label">Name <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required value={registerForm.name}
                    onChange={e => setRegisterForm(f => ({ ...f, name: e.target.value }))}
                    placeholder="us-east-1" />
                </div>
                <div>
                  <label className="label">Base URL <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required value={registerForm.baseUrl}
                    onChange={e => setRegisterForm(f => ({ ...f, baseUrl: e.target.value }))}
                    placeholder="https://regional-host:8080" />
                </div>
                <div>
                  <label className="label">Service token <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required type="password" value={registerForm.serviceToken}
                    onChange={e => setRegisterForm(f => ({ ...f, serviceToken: e.target.value }))}
                    placeholder="Token with device.read and artifact.read scopes" />
                </div>
                <div>
                  <label className="label">Sync interval (seconds)</label>
                  <input className="input" type="number" min="10" value={registerForm.syncIntervalSeconds}
                    onChange={e => setRegisterForm(f => ({ ...f, syncIntervalSeconds: e.target.value }))} />
                </div>
                <div>
                  <label className="label">TLS CA certificate (optional — for self-signed regional CPs)</label>
                  <textarea className="input" rows={4} value={registerForm.tlsCaPem}
                    onChange={e => setRegisterForm(f => ({ ...f, tlsCaPem: e.target.value }))}
                    placeholder="-----BEGIN CERTIFICATE-----&#10;..." style={{ fontFamily: 'monospace', fontSize: 11 }} />
                </div>
                {registerError && <div style={{ color: '#f27272', fontSize: 12 }}>{registerError}</div>}
              </div>
              <div className="modal-footer">
                <button type="button" className="button ghost" onClick={() => setRegisterOpen(false)}>Cancel</button>
                <button type="submit" className="button" disabled={registerSaving}>
                  {registerSaving ? 'Registering…' : 'Register'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </section>
  )
}
