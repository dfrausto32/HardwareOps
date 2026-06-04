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
  const policySyncAt = plane.lastPolicySyncAt
  const policySyncError = plane.lastPolicySyncError

  return (
    <div className="card" style={{ padding: '16px 20px', minWidth: 240 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <div style={{ fontWeight: 600, fontSize: 14 }}>{plane.name}</div>
        <div style={{ display: 'flex', gap: 4 }}>
          <span className={`chip ${syncError ? 'error' : 'success'}`} style={{ fontSize: 11 }}>
            {syncError ? 'sync error' : 'synced'}
          </span>
          {(policySyncAt || policySyncError) && (
            <span
              className={`chip ${policySyncError ? 'warning' : 'success'}`}
              style={{ fontSize: 11 }}
              title={policySyncError || 'Policies up to date'}
            >
              {policySyncError ? 'policy warn' : 'policy ok'}
            </span>
          )}
        </div>
      </div>
      <div className="muted" style={{ fontSize: 11, marginBottom: policySyncAt || policySyncError ? 4 : 10 }}>
        {plane.baseUrl} · data sync <RelativeTime iso={synced} />
      </div>
      {(policySyncAt || policySyncError) && (
        <div className="muted" style={{ fontSize: 11, marginBottom: 10 }}>
          policy sync{' '}
          {policySyncError
            ? <span style={{ color: '#f1c76f' }}>failed · last ok <RelativeTime iso={policySyncAt} /></span>
            : <RelativeTime iso={policySyncAt} />}
        </div>
      )}
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
      {policySyncError && (
        <div
          style={{
            fontSize: 11,
            color: '#f1c76f',
            marginBottom: 10,
            wordBreak: 'break-all',
            background: 'rgba(241,199,111,0.08)',
            borderRadius: 4,
            padding: '4px 8px',
          }}
        >
          Policy: {policySyncError}
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
  const [loginEmail, setLoginEmail] = useState('')
  const [loginPassword, setLoginPassword] = useState('')
  const [loginError, setLoginError] = useState(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [authStatus, setAuthStatus] = useState(null) // {enabled, oidcEnabled, oidcLoginUrl}
  const [planes, setPlanes] = useState([])
  const [healthMap, setHealthMap] = useState({}) // planeId → health snapshot
  const [devices, setDevices] = useState([])
  const [artifacts, setArtifacts] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [deviceFilter, setDeviceFilter] = useState('')
  const [artifactFilter, setArtifactFilter] = useState('')
  const [tab, setTab] = useState('health') // health | devices | artifacts | federated | groups
  const [registerOpen, setRegisterOpen] = useState(false)
  const [registerForm, setRegisterForm] = useState({ name: '', baseUrl: '', serviceToken: '', tlsCaPem: '', syncIntervalSeconds: '60' })
  const [registerError, setRegisterError] = useState(null)
  const [registerSaving, setRegisterSaving] = useState(false)
  const [fedArtifacts, setFedArtifacts] = useState([])
  const [fedFilter, setFedFilter] = useState('')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [uploadForm, setUploadForm] = useState({ name: '', version: '', type: 'app_bundle', file: null })
  const [uploadError, setUploadError] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [groups, setGroups] = useState([])
  const [desiredStates, setDesiredStates] = useState([]) // [{GlobalGroup, GlobalDesiredState}]
  const [groupOpen, setGroupOpen] = useState(false)
  const [groupForm, setGroupForm] = useState({ name: '', selectorJson: '{}' })
  const [groupError, setGroupError] = useState(null)
  const [groupSaving, setGroupSaving] = useState(false)
  const [policyOpen, setPolicyOpen] = useState(null) // groupId or null
  const [policyForm, setPolicyForm] = useState({ artifactId: '', desiredVersion: '', desiredConfigRev: '', checkinInterval: '0' })
  const [policyError, setPolicyError] = useState(null)
  const [policySaving, setPolicySaving] = useState(false)
  // E5 enrollment
  const [enrollmentProfiles, setEnrollmentProfiles] = useState([])
  const [pendingEnrollments, setPendingEnrollments] = useState([])
  const [profileOpen, setProfileOpen] = useState(false)
  const [profileForm, setProfileForm] = useState({ name: '', requireApproval: true, maxUses: '0', certValidityDays: '365' })
  const [profileError, setProfileError] = useState(null)
  const [profileSaving, setProfileSaving] = useState(false)
  const [enrollFilter, setEnrollFilter] = useState('')
  const pollRef = useRef(null)

  const effectiveUrl = globalPlaneUrl || urlInput

  // Check for OIDC token returned via redirect (?global_oidc_token=...).
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const oidcTok = params.get('global_oidc_token')
    if (oidcTok) {
      try { window.localStorage.setItem('hwops_global_token', oidcTok) } catch {}
      setToken(oidcTok)
      params.delete('global_oidc_token')
      const newSearch = params.toString()
      window.history.replaceState(null, '', newSearch ? '?' + newSearch : window.location.pathname)
    }
  }, [])

  // Fetch auth status from the global-plane to know if OIDC is available.
  useEffect(() => {
    if (!effectiveUrl) return
    fetch(`${effectiveUrl}/api/v1/auth/status`)
      .then(r => r.json())
      .then(setAuthStatus)
      .catch(() => {})
  }, [effectiveUrl])

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
      const [planesData, healthData, devicesData, artifactsData, fedData, groupsData, dsData, epData, peData] = await Promise.all([
        apiFetch('/api/v1/planes'),
        apiFetch('/api/v1/health/summary'),
        apiFetch('/api/v1/devices'),
        apiFetch('/api/v1/artifacts'),
        apiFetch('/api/v1/federation/artifacts').catch(() => []),
        apiFetch('/api/v1/groups').catch(() => []),
        apiFetch('/api/v1/desired-state').catch(() => []),
        apiFetch('/api/v1/enrollment-profiles').catch(() => []),
        apiFetch('/api/v1/pending-enrollments').catch(() => []),
      ])
      setPlanes(planesData || [])
      const hm = {}
      if (healthData?.planes) {
        for (const ph of healthData.planes) hm[ph.planeId] = ph
      }
      setHealthMap(hm)
      setDevices(devicesData || [])
      setArtifacts(artifactsData || [])
      setFedArtifacts(fedData || [])
      setGroups(groupsData || [])
      setDesiredStates(dsData || [])
      setEnrollmentProfiles(epData || [])
      setPendingEnrollments(peData || [])
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

  const handleLogin = async (e) => {
    e.preventDefault()
    setLoginError(null)
    setLoginLoading(true)
    try {
      const url = globalPlaneUrl || urlInput
      if (!url) throw new Error('Set the global-plane URL first')
      const resp = await fetch(`${url}/api/v1/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: loginEmail, password: loginPassword }),
      })
      const data = await resp.json()
      if (!resp.ok) throw new Error(data || `${resp.status}`)
      if (!data.token) throw new Error('No token in response')
      try { window.localStorage.setItem('hwops_global_token', data.token) } catch {}
      setToken(data.token)
      setLoginPassword('')
      setLoginError(null)
      if (onChangeGlobalPlaneUrl && urlInput) onChangeGlobalPlaneUrl(urlInput)
    } catch (err) {
      setLoginError(err.message)
    } finally {
      setLoginLoading(false)
    }
  }

  const handleSignOut = () => {
    try { window.localStorage.removeItem('hwops_global_token') } catch {}
    setToken('')
    setLoginEmail('')
    setLoginPassword('')
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
            <>
              <button className="button ghost" onClick={() => setUploadOpen(true)}>
                Upload artifact
              </button>
              <button className="button" onClick={() => setRegisterOpen(true)}>
                Register plane
              </button>
            </>
          )}
        </div>
      </div>

      {/* Connection config */}
      <div className="card" style={{ padding: '14px 18px', marginBottom: 20 }}>
        <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 10 }}>Global-plane connection</div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end', marginBottom: 10 }}>
          <div style={{ flex: '1 1 260px' }}>
            <label className="label">URL</label>
            <input
              className="input"
              placeholder="https://global-plane-host:8090"
              value={urlInput}
              onChange={e => setUrlInput(e.target.value)}
            />
          </div>
        </div>

        {/* Login form — shown when no token is active */}
        {!token ? (
          <>
            {authStatus?.oidcEnabled && (
              <div style={{ marginBottom: 10 }}>
                <a
                  className="button"
                  href={`${effectiveUrl}/api/v1/auth/oidc/login`}
                  style={{ display: 'inline-block' }}
                >
                  Sign in with SSO
                </a>
              </div>
            )}
            <form onSubmit={handleLogin} style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end', marginBottom: 4 }}>
              <div style={{ flex: '1 1 180px' }}>
                <label className="label">Email</label>
                <input
                  className="input"
                  type="email"
                  placeholder="operator@example.com"
                  value={loginEmail}
                  onChange={e => setLoginEmail(e.target.value)}
                  autoComplete="username"
                />
              </div>
              <div style={{ flex: '1 1 160px' }}>
                <label className="label">Password</label>
                <input
                  className="input"
                  type="password"
                  placeholder="Password"
                  value={loginPassword}
                  onChange={e => setLoginPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </div>
              <button className="button" type="submit" disabled={loginLoading}>
                {loginLoading ? 'Signing in…' : 'Sign in'}
              </button>
            </form>
          </>
        ) : (
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 4 }}>
            <span className="chip success" style={{ fontSize: 12 }}>Authenticated</span>
            <button className="button ghost" style={{ fontSize: 12 }} onClick={handleSignOut}>Sign out</button>
          </div>
        )}

        {/* Service-token / break-glass fallback */}
        <details style={{ marginTop: 8 }}>
          <summary className="muted" style={{ fontSize: 12, cursor: 'pointer' }}>Use service token (break-glass)</summary>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end', marginTop: 8 }}>
            <div style={{ flex: '1 1 260px' }}>
              <input
                className="input"
                type="password"
                placeholder="JWT or service token"
                value={token}
                onChange={e => setToken(e.target.value)}
              />
            </div>
            <button className="button ghost" onClick={handleConnect}>Set token</button>
          </div>
        </details>

        {loginError && <div style={{ color: '#f27272', marginTop: 8, fontSize: 12 }}>{loginError}</div>}
        {error && <div style={{ color: '#f27272', marginTop: 4, fontSize: 12 }}>{error}</div>}
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
            {[['health', `Planes (${planes.length})`], ['devices', `Devices (${devices.length})`], ['artifacts', `Artifacts (${artifacts.length})`], ['federated', `Federated (${fedArtifacts.length})`], ['groups', `Groups (${groups.length})`], ['enrollments', `Enrollments (${pendingEnrollments.filter(e => e.status === 'pending').length})`]].map(([id, label]) => (
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

          {/* Federated artifacts tab */}
          {tab === 'federated' && (
            <>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12, alignItems: 'center' }}>
                <input
                  className="input"
                  placeholder="Filter by name or version…"
                  value={fedFilter}
                  onChange={e => setFedFilter(e.target.value)}
                  style={{ maxWidth: 320 }}
                />
                <span className="muted" style={{ fontSize: 12 }}>
                  Artifacts uploaded to the global plane and replicated to regional planes.
                </span>
              </div>
              <table className="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Version</th>
                    <th>Type</th>
                    <th>Status</th>
                    <th>SHA-256</th>
                    <th>Size</th>
                    <th>Regions</th>
                    <th>Uploaded</th>
                  </tr>
                </thead>
                <tbody>
                  {fedArtifacts.filter(a => {
                    if (!fedFilter) return true
                    const q = fedFilter.toLowerCase()
                    return a.name?.toLowerCase().includes(q) || a.version?.toLowerCase().includes(q)
                  }).length === 0 ? (
                    <tr><td colSpan={8} style={{ textAlign: 'center', padding: '24px 0' }} className="muted">
                      No federated artifacts. Click <strong>Upload artifact</strong> to publish one to all regions.
                    </td></tr>
                  ) : fedArtifacts.filter(a => {
                    if (!fedFilter) return true
                    const q = fedFilter.toLowerCase()
                    return a.name?.toLowerCase().includes(q) || a.version?.toLowerCase().includes(q)
                  }).map(a => {
                    const sizeKb = a.sizeBytes ? `${Math.round(a.sizeBytes / 1024)} KB` : '—'
                    const sha = a.sha256 ? a.sha256.slice(0, 12) + '…' : '—'
                    const confirmed = a.confirmedRegions ?? 0
                    const total = a.totalRegions ?? 0
                    const allConfirmed = total > 0 && confirmed === total
                    return (
                      <tr key={a.artifactId}>
                        <td style={{ fontWeight: 500 }}>{a.name || '—'}</td>
                        <td><code style={{ fontSize: 12 }}>{a.version || '—'}</code></td>
                        <td className="muted" style={{ fontSize: 12 }}>{a.artifactType || '—'}</td>
                        <td>
                          <span className={`chip ${a.status === 'active' ? 'success' : ''}`} style={{ fontSize: 11 }}>
                            {a.status || '—'}
                          </span>
                        </td>
                        <td><code style={{ fontSize: 11 }}>{sha}</code></td>
                        <td className="muted" style={{ fontSize: 12 }}>{sizeKb}</td>
                        <td>
                          <span
                            className={`chip ${allConfirmed ? 'success' : total === 0 ? '' : 'warning'}`}
                            style={{ fontSize: 11 }}
                            title={`${confirmed} of ${total} regions confirmed`}
                          >
                            {confirmed}/{total}
                          </span>
                        </td>
                        <td className="muted" style={{ fontSize: 12 }}><RelativeTime iso={a.createdAt} /></td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </>
          )}

          {/* Global groups tab */}
          {tab === 'groups' && (
            <>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12, alignItems: 'center' }}>
                <button className="button" onClick={() => setGroupOpen(true)}>Create group</button>
                <span className="muted" style={{ fontSize: 12 }}>Global groups define label selectors for cross-region policy assignment.</span>
              </div>
              {desiredStates.length === 0 && groups.length === 0 ? (
                <div className="muted" style={{ textAlign: 'center', padding: '32px 0' }}>
                  No global groups yet. Click <strong>Create group</strong> to define a label-based device group.
                </div>
              ) : (
                <table className="table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Selector</th>
                      <th>Artifact ID</th>
                      <th>Version</th>
                      <th>Checkin interval</th>
                      <th>Updated</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {desiredStates.map(row => (
                      <tr key={row.groupId}>
                        <td style={{ fontWeight: 500 }}>{row.name || '—'}</td>
                        <td><code style={{ fontSize: 11 }}>{row.selectorJson ? JSON.stringify(row.selectorJson) : '{}'}</code></td>
                        <td><code style={{ fontSize: 11 }}>{row.artifactId || <span className="muted">—</span>}</code></td>
                        <td>{row.desiredVersion || <span className="muted">—</span>}</td>
                        <td className="muted" style={{ fontSize: 12 }}>{row.checkinInterval ? `${row.checkinInterval}s` : '—'}</td>
                        <td className="muted" style={{ fontSize: 12 }}><RelativeTime iso={row.updatedAt} /></td>
                        <td>
                          <button
                            className="button ghost"
                            style={{ fontSize: 12, padding: '2px 8px' }}
                            onClick={() => {
                              setPolicyOpen(row.groupId)
                              setPolicyForm({
                                artifactId: row.artifactId || '',
                                desiredVersion: row.desiredVersion || '',
                                desiredConfigRev: row.desiredConfigRev || '',
                                checkinInterval: String(row.checkinInterval || 0),
                              })
                              setPolicyError(null)
                            }}
                          >
                            Set policy
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </>
          )}
          {/* Enrollments tab */}
          {tab === 'enrollments' && (
            <>
              {/* Enrollment profiles section */}
              <div style={{ display: 'flex', gap: 8, marginBottom: 12, alignItems: 'center' }}>
                <button className="button" onClick={() => setProfileOpen(true)}>Create profile</button>
                <span className="muted" style={{ fontSize: 12 }}>
                  Enrollment profiles define how devices join this deployment.
                </span>
              </div>
              {enrollmentProfiles.length > 0 && (
                <table className="table" style={{ marginBottom: 24 }}>
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Approval</th>
                      <th>Max uses</th>
                      <th>Cert validity</th>
                      <th>Status</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {enrollmentProfiles.map(p => (
                      <tr key={p.profileId}>
                        <td style={{ fontWeight: 500 }}>{p.name}</td>
                        <td>
                          <span className={`chip ${p.requireApproval ? 'warning' : 'success'}`} style={{ fontSize: 11 }}>
                            {p.requireApproval ? 'manual' : 'auto'}
                          </span>
                        </td>
                        <td className="muted" style={{ fontSize: 12 }}>{p.maxUses || '∞'}</td>
                        <td className="muted" style={{ fontSize: 12 }}>{p.certValidityDays}d</td>
                        <td>
                          <span className={`chip ${p.disabled ? 'error' : 'success'}`} style={{ fontSize: 11 }}>
                            {p.disabled ? 'disabled' : 'active'}
                          </span>
                        </td>
                        <td>
                          <button
                            className="button ghost"
                            style={{ fontSize: 12, padding: '2px 8px' }}
                            onClick={async () => {
                              if (!window.confirm(`Delete profile "${p.name}"?`)) return
                              try {
                                await apiFetch(`/api/v1/enrollment-profiles/${p.profileId}`, { method: 'DELETE' })
                                loadAll()
                              } catch (err) {
                                alert(err.message)
                              }
                            }}
                          >
                            Delete
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}

              {/* Pending enrollments section */}
              <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 8 }}>
                Pending enrollments — unified queue
              </div>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                <input
                  className="input"
                  placeholder="Filter by request ID, hardware ID or region…"
                  value={enrollFilter}
                  onChange={e => setEnrollFilter(e.target.value)}
                  style={{ maxWidth: 360 }}
                />
              </div>
              {pendingEnrollments.filter(e => {
                if (!enrollFilter) return true
                const q = enrollFilter.toLowerCase()
                return e.requestId?.toLowerCase().includes(q) ||
                  e.hardwareId?.toLowerCase().includes(q) ||
                  e.planeName?.toLowerCase().includes(q)
              }).length === 0 ? (
                <div className="muted" style={{ textAlign: 'center', padding: '24px 0' }}>
                  No pending enrollments found.
                </div>
              ) : (
                <table className="table">
                  <thead>
                    <tr>
                      <th>Request ID</th>
                      <th>Region</th>
                      <th>Status</th>
                      <th>Hardware ID</th>
                      <th>Agent</th>
                      <th>Source IP</th>
                      <th>Created</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {pendingEnrollments.filter(e => {
                      if (!enrollFilter) return true
                      const q = enrollFilter.toLowerCase()
                      return e.requestId?.toLowerCase().includes(q) ||
                        e.hardwareId?.toLowerCase().includes(q) ||
                        e.planeName?.toLowerCase().includes(q)
                    }).map(e => (
                      <tr key={`${e.planeId}:${e.requestId}`}>
                        <td><code style={{ fontSize: 11 }}>{e.requestId?.slice(0, 12)}…</code></td>
                        <td className="muted" style={{ fontSize: 12 }}>{e.planeName || e.planeId}</td>
                        <td>
                          <span className={`chip ${e.status === 'pending' ? 'warning' : e.status === 'approved' ? 'success' : 'error'}`} style={{ fontSize: 11 }}>
                            {e.status}
                          </span>
                        </td>
                        <td className="muted" style={{ fontSize: 12 }}>{e.hardwareId || '—'}</td>
                        <td className="muted" style={{ fontSize: 12 }}>{e.agentVersion || '—'}</td>
                        <td className="muted" style={{ fontSize: 12 }}>{e.sourceIp || '—'}</td>
                        <td className="muted" style={{ fontSize: 12 }}><RelativeTime iso={e.createdAt} /></td>
                        <td>
                          {e.status === 'pending' && (
                            <div style={{ display: 'flex', gap: 4 }}>
                              <button
                                className="button"
                                style={{ fontSize: 11, padding: '2px 8px' }}
                                onClick={async () => {
                                  try {
                                    await apiFetch(`/api/v1/pending-enrollments/${e.requestId}/approve`, { method: 'POST' })
                                    loadAll()
                                  } catch (err) { alert(err.message) }
                                }}
                              >
                                Approve
                              </button>
                              <button
                                className="button ghost"
                                style={{ fontSize: 11, padding: '2px 8px' }}
                                onClick={async () => {
                                  const reason = window.prompt('Deny reason (optional):') ?? ''
                                  try {
                                    await apiFetch(`/api/v1/pending-enrollments/${e.requestId}/deny`, {
                                      method: 'POST',
                                      body: JSON.stringify({ reason }),
                                    })
                                    loadAll()
                                  } catch (err) { alert(err.message) }
                                }}
                              >
                                Deny
                              </button>
                            </div>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </>
          )}
        </>
      )}

      {/* Create enrollment profile modal */}
      {profileOpen && (
        <div className="modal-overlay" onClick={() => setProfileOpen(false)}>
          <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 460 }}>
            <div className="modal-header">
              <h3>Create enrollment profile</h3>
              <button className="modal-close" onClick={() => setProfileOpen(false)}>×</button>
            </div>
            <form onSubmit={async (e) => {
              e.preventDefault()
              setProfileError(null)
              setProfileSaving(true)
              try {
                await apiFetch('/api/v1/enrollment-profiles', {
                  method: 'POST',
                  body: JSON.stringify({
                    name: profileForm.name,
                    requireApproval: profileForm.requireApproval,
                    maxUses: parseInt(profileForm.maxUses, 10) || 0,
                    certValidityDays: parseInt(profileForm.certValidityDays, 10) || 365,
                  }),
                })
                setProfileOpen(false)
                setProfileForm({ name: '', requireApproval: true, maxUses: '0', certValidityDays: '365' })
                loadAll()
              } catch (err) {
                setProfileError(err.message)
              } finally {
                setProfileSaving(false)
              }
            }}>
              <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div>
                  <label className="label">Name <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required value={profileForm.name}
                    onChange={e => setProfileForm(f => ({ ...f, name: e.target.value }))}
                    placeholder="default-profile" />
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                  <input type="checkbox" id="requireApproval" checked={profileForm.requireApproval}
                    onChange={e => setProfileForm(f => ({ ...f, requireApproval: e.target.checked }))} />
                  <label htmlFor="requireApproval" style={{ fontSize: 13 }}>Require manual approval</label>
                </div>
                <div style={{ display: 'flex', gap: 12 }}>
                  <div style={{ flex: 1 }}>
                    <label className="label">Max uses (0 = unlimited)</label>
                    <input className="input" type="number" min="0" value={profileForm.maxUses}
                      onChange={e => setProfileForm(f => ({ ...f, maxUses: e.target.value }))} />
                  </div>
                  <div style={{ flex: 1 }}>
                    <label className="label">Cert validity (days)</label>
                    <input className="input" type="number" min="1" value={profileForm.certValidityDays}
                      onChange={e => setProfileForm(f => ({ ...f, certValidityDays: e.target.value }))} />
                  </div>
                </div>
                {profileError && <div style={{ color: '#f27272', fontSize: 12 }}>{profileError}</div>}
              </div>
              <div className="modal-footer">
                <button type="button" className="button ghost" onClick={() => setProfileOpen(false)}>Cancel</button>
                <button type="submit" className="button" disabled={profileSaving}>
                  {profileSaving ? 'Creating…' : 'Create profile'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Upload federated artifact modal */}
      {uploadOpen && (
        <div className="modal-overlay" onClick={() => setUploadOpen(false)}>
          <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 460 }}>
            <div className="modal-header">
              <h3>Upload federated artifact</h3>
              <button className="modal-close" onClick={() => setUploadOpen(false)}>×</button>
            </div>
            <form onSubmit={async (e) => {
              e.preventDefault()
              setUploadError(null)
              setUploading(true)
              try {
                const fd = new FormData()
                fd.append('name', uploadForm.name)
                fd.append('version', uploadForm.version)
                fd.append('type', uploadForm.type)
                fd.append('file', uploadForm.file)
                const headers = {}
                if (token) headers['Authorization'] = `Bearer ${token}`
                const resp = await fetch(`${effectiveUrl}/api/v1/federation/artifacts/upload`, {
                  method: 'POST',
                  headers,
                  body: fd,
                })
                if (!resp.ok) {
                  const text = await resp.text()
                  throw new Error(`${resp.status}: ${text.slice(0, 200)}`)
                }
                setUploadOpen(false)
                setUploadForm({ name: '', version: '', type: 'app_bundle', file: null })
                setTab('federated')
                loadAll()
              } catch (err) {
                setUploadError(err.message)
              } finally {
                setUploading(false)
              }
            }}>
              <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div>
                  <label className="label">Name <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required value={uploadForm.name}
                    onChange={e => setUploadForm(f => ({ ...f, name: e.target.value }))}
                    placeholder="my-app" />
                </div>
                <div>
                  <label className="label">Version <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required value={uploadForm.version}
                    onChange={e => setUploadForm(f => ({ ...f, version: e.target.value }))}
                    placeholder="1.0.0" />
                </div>
                <div>
                  <label className="label">Type <span style={{ color: '#f27272' }}>*</span></label>
                  <select className="input" value={uploadForm.type}
                    onChange={e => setUploadForm(f => ({ ...f, type: e.target.value }))}>
                    <option value="app_bundle">app_bundle</option>
                    <option value="config_bundle">config_bundle</option>
                    <option value="data_bundle">data_bundle</option>
                    <option value="firmware">firmware</option>
                    <option value="container_image">container_image</option>
                  </select>
                </div>
                <div>
                  <label className="label">File (tar.gz) <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required type="file" accept=".tar.gz,.tgz"
                    onChange={e => setUploadForm(f => ({ ...f, file: e.target.files[0] || null }))} />
                </div>
                {uploadError && <div style={{ color: '#f27272', fontSize: 12 }}>{uploadError}</div>}
              </div>
              <div className="modal-footer">
                <button type="button" className="button ghost" onClick={() => setUploadOpen(false)}>Cancel</button>
                <button type="submit" className="button" disabled={uploading || !uploadForm.file}>
                  {uploading ? 'Uploading…' : 'Upload & replicate'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Create global group modal */}
      {groupOpen && (
        <div className="modal-overlay" onClick={() => setGroupOpen(false)}>
          <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 460 }}>
            <div className="modal-header">
              <h3>Create global group</h3>
              <button className="modal-close" onClick={() => setGroupOpen(false)}>×</button>
            </div>
            <form onSubmit={async (e) => {
              e.preventDefault()
              setGroupError(null)
              setGroupSaving(true)
              try {
                let sel
                try { sel = JSON.parse(groupForm.selectorJson) } catch { throw new Error('Selector JSON is invalid') }
                await apiFetch('/api/v1/groups', {
                  method: 'POST',
                  body: JSON.stringify({ name: groupForm.name, selectorJson: sel }),
                })
                setGroupOpen(false)
                setGroupForm({ name: '', selectorJson: '{}' })
                setTab('groups')
                loadAll()
              } catch (err) {
                setGroupError(err.message)
              } finally {
                setGroupSaving(false)
              }
            }}>
              <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div>
                  <label className="label">Name <span style={{ color: '#f27272' }}>*</span></label>
                  <input className="input" required value={groupForm.name}
                    onChange={e => setGroupForm(f => ({ ...f, name: e.target.value }))}
                    placeholder="fleet-prod" />
                </div>
                <div>
                  <label className="label">Label selector (JSON)</label>
                  <textarea className="input" rows={3} value={groupForm.selectorJson}
                    onChange={e => setGroupForm(f => ({ ...f, selectorJson: e.target.value }))}
                    placeholder='{"env":"prod","region":"us-east-1"}'
                    style={{ fontFamily: 'monospace', fontSize: 12 }} />
                  <div className="muted" style={{ fontSize: 11, marginTop: 4 }}>
                    Devices whose labels contain all keys from this object will be matched.
                  </div>
                </div>
                {groupError && <div style={{ color: '#f27272', fontSize: 12 }}>{groupError}</div>}
              </div>
              <div className="modal-footer">
                <button type="button" className="button ghost" onClick={() => setGroupOpen(false)}>Cancel</button>
                <button type="submit" className="button" disabled={groupSaving}>
                  {groupSaving ? 'Creating…' : 'Create group'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Set global policy modal */}
      {policyOpen && (
        <div className="modal-overlay" onClick={() => setPolicyOpen(null)}>
          <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 480 }}>
            <div className="modal-header">
              <h3>Set global policy</h3>
              <button className="modal-close" onClick={() => setPolicyOpen(null)}>×</button>
            </div>
            <form onSubmit={async (e) => {
              e.preventDefault()
              setPolicyError(null)
              setPolicySaving(true)
              try {
                await apiFetch(`/api/v1/groups/${policyOpen}/desired-state`, {
                  method: 'PUT',
                  body: JSON.stringify({
                    artifactId: policyForm.artifactId,
                    desiredVersion: policyForm.desiredVersion,
                    desiredConfigRev: policyForm.desiredConfigRev,
                    checkinInterval: parseInt(policyForm.checkinInterval, 10) || 0,
                  }),
                })
                setPolicyOpen(null)
                loadAll()
              } catch (err) {
                setPolicyError(err.message)
              } finally {
                setPolicySaving(false)
              }
            }}>
              <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div className="muted" style={{ fontSize: 12 }}>
                  This policy will be pushed to all enabled regional planes and applied to matching devices.
                </div>
                <div>
                  <label className="label">Artifact ID</label>
                  <input className="input" value={policyForm.artifactId}
                    onChange={e => setPolicyForm(f => ({ ...f, artifactId: e.target.value }))}
                    placeholder="Leave blank to inherit from group" />
                </div>
                <div>
                  <label className="label">Desired version</label>
                  <input className="input" value={policyForm.desiredVersion}
                    onChange={e => setPolicyForm(f => ({ ...f, desiredVersion: e.target.value }))}
                    placeholder="1.2.3" />
                </div>
                <div>
                  <label className="label">Desired config rev</label>
                  <input className="input" value={policyForm.desiredConfigRev}
                    onChange={e => setPolicyForm(f => ({ ...f, desiredConfigRev: e.target.value }))}
                    placeholder="abc123" />
                </div>
                <div>
                  <label className="label">Checkin interval (seconds, 0 = default)</label>
                  <input className="input" type="number" min="0" value={policyForm.checkinInterval}
                    onChange={e => setPolicyForm(f => ({ ...f, checkinInterval: e.target.value }))} />
                </div>
                {policyError && <div style={{ color: '#f27272', fontSize: 12 }}>{policyError}</div>}
              </div>
              <div className="modal-footer">
                <button type="button" className="button ghost" onClick={() => setPolicyOpen(null)}>Cancel</button>
                <button type="submit" className="button" disabled={policySaving}>
                  {policySaving ? 'Saving…' : 'Save policy'}
                </button>
              </div>
            </form>
          </div>
        </div>
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
