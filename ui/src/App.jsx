import { useEffect, useMemo, useState } from 'react'
import {
  getDevices,
  getDevice,
  getDesiredState,
  listArtifacts,
  uploadArtifact,
  setDesiredStateDevice,
  getDeviceLogs,
  deleteDevice,
  deleteArtifact,
} from './api'

const nav = [
  { id: 'dashboard', label: 'Dashboard', icon: 'icon-dashboard' },
  { id: 'logs', label: 'Logs', icon: 'icon-logs' },
]

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

export default function App() {
  const apiBaseUrl = useMemo(() => {
    return import.meta.env.VITE_API_BASE_URL || 'https://localhost:8080'
  }, [])
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
  const wsUrl = `${wsBaseUrl}/api/v1/events`

  const [view, setView] = useState(() => {
    const hash = window.location.hash.replace('#', '')
    return hash || 'dashboard'
  })

  useEffect(() => {
    const onHash = () => {
      const hash = window.location.hash.replace('#', '')
      setView(hash || 'dashboard')
    }
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const [devices, setDevices] = useState([])
  const [devicesLoading, setDevicesLoading] = useState(false)
  const [devicesError, setDevicesError] = useState('')
  const [devicesStatus, setDevicesStatus] = useState('')

  const [selectedDeviceId, setSelectedDeviceId] = useState('')
  const [deviceDetail, setDeviceDetail] = useState(null)
  const [deviceDetailError, setDeviceDetailError] = useState('')
  const [deviceDrawerOpen, setDeviceDrawerOpen] = useState(false)
  const [artifactModalOpen, setArtifactModalOpen] = useState(false)

  const [artifacts, setArtifacts] = useState([])
  const [artifactsError, setArtifactsError] = useState('')
  const [artifactsStatus, setArtifactsStatus] = useState('')

  const [desiredState, setDesiredState] = useState({ groups: [], devices: [] })
  const [desiredError, setDesiredError] = useState('')

  const [uploadStatus, setUploadStatus] = useState('')
  const [desiredStatus, setDesiredStatus] = useState('')

  const [deviceForm, setDeviceForm] = useState({
    deviceId: '',
    artifactId: '',
    desiredVersion: '',
    desiredConfigRev: '',
    checkinIntervalSec: '',
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

  const [eventsFeed, setEventsFeed] = useState([])
  const [eventsStatus, setEventsStatus] = useState('disconnected')
  const [eventsError, setEventsError] = useState('')

  const [theme, setTheme] = useState(() => {
    return localStorage.getItem('hwops-theme') || 'dark'
  })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('hwops-theme', theme)
  }, [theme])

  useEffect(() => {
    loadDevices()
    loadArtifacts()
    loadDesired()
  }, [])

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
    setDeviceForm({
      deviceId: selectedDeviceId,
      artifactId: desired?.artifactId || '',
      desiredVersion: desired?.desiredVersion || current?.softwareVersion || '',
      desiredConfigRev: desired?.desiredConfigRev || current?.configRev || '',
      checkinIntervalSec: desired?.checkinIntervalSec ? String(desired.checkinIntervalSec) : '',
    })
  }, [selectedDeviceId, desiredState, deviceDetail, deviceFormDirty])

  useEffect(() => {
    let ws
    let reconnectTimer
    let shouldReconnect = true

    const connect = () => {
      setEventsStatus('connecting')
      setEventsError('')
      ws = new WebSocket(wsUrl)
      ws.onopen = () => setEventsStatus('connected')
      ws.onmessage = (evt) => {
        try {
          const data = JSON.parse(evt.data)
          setEventsFeed((prev) => [data, ...prev].slice(0, 200))
        } catch (err) {
          setEventsError('Failed to parse event payload')
        }
      }
      ws.onerror = () => {
        setEventsStatus('disconnected')
      }
      ws.onclose = () => {
        setEventsStatus('disconnected')
        if (shouldReconnect) {
          reconnectTimer = setTimeout(connect, 2000)
        }
      }
    }

    connect()
    return () => {
      shouldReconnect = false
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (ws) ws.close()
    }
  }, [wsUrl])

  const dashboard = useMemo(() => {
    const total = devices.length
    const active = devices.filter((d) => d.status === 'active').length
    const lastSeen = devices
      .map((d) => (d.lastSeen ? new Date(d.lastSeen) : null))
      .filter(Boolean)
      .sort((a, b) => b - a)[0]
    const lastSeenLabel = lastSeen ? lastSeen.toISOString() : '—'
    const lastApplyStatus = deviceDetail?.current?.lastApplyStatus || '—'
    const lastPreApplyStatus = deviceDetail?.current?.lastPreApplyStatus || '—'
    return { total, active, lastSeenLabel, lastApplyStatus, lastPreApplyStatus }
  }, [devices, deviceDetail])

  const preApplyBadgeClass = useMemo(() => {
    const raw = (dashboard.lastPreApplyStatus || '').toLowerCase()
    if (!raw || raw === '—') return 'unknown'
    return raw
  }, [dashboard.lastPreApplyStatus])

  const notifications = useMemo(() => {
    const items = []
    if (devicesError) items.push({ type: 'error', text: devicesError })
    if (artifactsError) items.push({ type: 'error', text: artifactsError })
    if (desiredError) items.push({ type: 'error', text: desiredError })
    if (eventsError) items.push({ type: 'error', text: eventsError })
    if (uploadStatus) items.push({ type: 'info', text: uploadStatus })
    if (devicesStatus) items.push({ type: 'info', text: devicesStatus })
    if (artifactsStatus) items.push({ type: 'info', text: artifactsStatus })
    if (desiredStatus) items.push({ type: 'info', text: desiredStatus })
    return items.slice(0, 4)
  }, [devicesError, artifactsError, desiredError, eventsError, uploadStatus, devicesStatus, artifactsStatus, desiredStatus])

  function loadDevices() {
    setDevicesLoading(true)
    setDevicesError('')
    getDevices()
      .then((res) => {
        const items = res.items || []
        setDevices(items)
        if (!selectedDeviceId && items.length > 0) {
          const latest = [...items].sort((a, b) => {
            const at = a.lastSeen ? new Date(a.lastSeen).getTime() : 0
            const bt = b.lastSeen ? new Date(b.lastSeen).getTime() : 0
            return bt - at
          })[0]
          setSelectedDeviceId(latest.deviceId)
        }
      })
      .catch((err) => setDevicesError(err.message || String(err)))
      .finally(() => setDevicesLoading(false))
  }

  function loadArtifacts() {
    setArtifactsError('')
    listArtifacts()
      .then((res) => setArtifacts(res.items || []))
      .catch((err) => setArtifactsError(err.message || String(err)))
  }

  function loadDesired() {
    setDesiredError('')
    getDesiredState()
      .then((res) => setDesiredState(res || { groups: [], devices: [] }))
      .catch((err) => setDesiredError(err.message || String(err)))
  }

  function handleUpload(e) {
    e.preventDefault()
    const form = e.currentTarget
    const formData = new FormData(form)
    setUploadStatus('Uploading...')
    uploadArtifact(formData)
      .then((resp) => {
        setUploadStatus(`Uploaded artifact ${resp.artifactId}`)
        form.reset()
        loadArtifacts()
      })
      .catch((err) => setUploadStatus(err.message || String(err)))
  }

  function handleDesiredDevice(e) {
    e.preventDefault()
    setDesiredStatus('Setting desired state...')
    const payload = {
      artifactId: deviceForm.artifactId || undefined,
      desiredVersion: deviceForm.desiredVersion || undefined,
      desiredConfigRev: deviceForm.desiredConfigRev || undefined,
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

  async function handleDeleteDevice(deviceId) {
    const ok = window.confirm(`Delete device ${deviceId}? This will remove desired state and apply history.`)
    if (!ok) return
    setDevicesStatus('Deleting device...')
    try {
      await deleteDevice(deviceId)
      setDevicesStatus(`Deleted device ${deviceId}`)
      if (selectedDeviceId === deviceId) {
        setSelectedDeviceId('')
        setDeviceDetail(null)
      }
      loadDevices()
      loadDesired()
    } catch (err) {
      setDevicesError(err.message || String(err))
    }
  }

  async function handleDeleteArtifact(artifactId) {
    const ok = window.confirm(`Delete artifact ${artifactId}? This will clear desired state references.`)
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

  const selectedDesired = desiredState.devices?.find((d) => d.deviceId === selectedDeviceId)
  const selectedArtifact = artifacts.find((a) => a.artifactId === deviceForm.artifactId)
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

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">HardwareOps</div>
        <nav className="nav">
          {nav.map((item) => (
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
        {view === 'dashboard' && (
          <>
            <section id="dashboard" className="card">
              <div className="section-header">
                <h2>Dashboard</h2>
                <div className="events-meta">
                  <span className={`pill ${eventsStatus}`}>{eventsStatus}</span>
                </div>
              </div>
              {notifications.length > 0 && (
                <div className="notices">
                  {notifications.map((n, idx) => (
                    <div key={`${n.type}-${idx}`} className={`notice ${n.type}`}>
                      {n.text}
                    </div>
                  ))}
                </div>
              )}
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
                  <div className="metric-label">Last Check-in</div>
                  <div className="metric-value small">{dashboard.lastSeenLabel}</div>
                </div>
                <div className="metric">
                  <div className="metric-label">Last Apply Status</div>
                  <div className="metric-value">{dashboard.lastApplyStatus}</div>
                  <div className="metric-sub">
                    <span className={`pill preapply ${preApplyBadgeClass}`}>
                      pre-apply {dashboard.lastPreApplyStatus}
                    </span>
                  </div>
                </div>
              </div>
              <div className="events">
                <h3>Live Events</h3>
                {eventsError && <div className="error">{eventsError}</div>}
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
              </div>
            </section>

            <section id="devices" className="card">
              <div className="section-header">
                <h2>Devices</h2>
                <button onClick={loadDevices} className="button">Refresh</button>
              </div>
              {devicesError && <div className="error">{devicesError}</div>}
              {devicesLoading ? (
                <div className="placeholder">Loading devices...</div>
              ) : (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Device ID</th>
                        <th>Status</th>
                        <th>Last Seen</th>
                        <th>Labels</th>
                      </tr>
                    </thead>
                    <tbody>
                      {devices.map((d) => (
                        <tr
                          key={d.deviceId}
                          className={selectedDeviceId === d.deviceId ? 'selected' : ''}
                          onClick={() => {
                            setSelectedDeviceId(d.deviceId)
                            setDeviceDrawerOpen(true)
                            setDeviceFormDirty(false)
                          }}
                        >
                          <td>{d.deviceId}</td>
                          <td>{d.status || '—'}</td>
                          <td>{d.lastSeen || '—'}</td>
                          <td><code>{d.labels ? JSON.stringify(d.labels) : '—'}</code></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            <section id="artifacts" className="card">
          <div className="section-header">
            <h2>Artifacts</h2>
            <button onClick={loadArtifacts} className="button">Refresh</button>
          </div>
          {artifactsError && <div className="error">{artifactsError}</div>}

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
            <div className="full">
              <button className="button" type="submit">Upload</button>
              {uploadStatus && <span className="status">{uploadStatus}</span>}
            </div>
          </form>

          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Artifact ID</th>
                  <th>Name</th>
                  <th>Version</th>
                  <th>SHA256</th>
                  <th>Size</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                  {artifacts.map((a) => (
                    <tr key={a.artifactId}>
                      <td>{a.artifactId}</td>
                      <td>{a.name}</td>
                      <td>{a.version}</td>
                      <td className="mono">{a.sha256}</td>
                      <td>{a.sizeBytes}</td>
                      <td>{a.createdAt}</td>
                      <td>
                        <button className="button ghost" onClick={() => handleDeleteArtifact(a.artifactId)}>
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            </section>
          </>
        )}

        {view === 'logs' && (
          <section id="logs" className="card">
          <div className="section-header">
            <h2>Logs</h2>
            <div className="logs-actions">
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
            </div>
          </div>

          <div className="form inline">
            <label>Device ID</label>
            <input
              value={logsDeviceId}
              onChange={(e) => setLogsDeviceId(e.target.value)}
              placeholder="device uuid"
            />
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
          </section>
        )}
      </main>

      {deviceDrawerOpen && (
        <div className="drawer-backdrop" onClick={() => setDeviceDrawerOpen(false)}>
          <div className="drawer" onClick={(e) => e.stopPropagation()}>
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

                <form
                  className="form"
                  onSubmit={(e) => {
                    handleDesiredDevice(e)
                  }}
                >
                  <h3>Desired State</h3>
                  {desiredError && <div className="error">{desiredError}</div>}
                  <label>Device ID</label>
                  <input value={deviceForm.deviceId} readOnly />
                  <label>Artifact</label>
                  <div className="inline-row">
                    <input
                      value={deviceForm.artifactId}
                      onChange={(e) => {
                        setDeviceForm({ ...deviceForm, artifactId: e.target.value })
                        setDeviceFormDirty(true)
                      }}
                      placeholder="artifact uuid"
                    />
                    <button className="button ghost" type="button" onClick={() => setArtifactModalOpen(true)}>
                      Select
                    </button>
                  </div>
                  {selectedArtifact && (
                    <div className="artifact-summary">
                      <div><strong>{selectedArtifact.name}</strong> v{selectedArtifact.version}</div>
                      <div className="mono">{selectedArtifact.artifactId}</div>
                    </div>
                  )}
                  <label>Desired Version</label>
                  <input
                    value={deviceForm.desiredVersion}
                    onChange={(e) => {
                      setDeviceForm({ ...deviceForm, desiredVersion: e.target.value })
                      setDeviceFormDirty(true)
                    }}
                    placeholder="1.0.0"
                  />
                  <label>Config Rev</label>
                  <input
                    value={deviceForm.desiredConfigRev}
                    onChange={(e) => {
                      setDeviceForm({ ...deviceForm, desiredConfigRev: e.target.value })
                      setDeviceFormDirty(true)
                    }}
                    placeholder="c1"
                  />
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
                    <button
                      className="button ghost"
                      type="button"
                      onClick={() => {
                        setDeviceFormDirty(false)
                        setDeviceForm({
                          deviceId: selectedDeviceId,
                          artifactId: selectedDesired?.artifactId || '',
                          desiredVersion: selectedDesired?.desiredVersion || deviceDetail.current?.softwareVersion || '',
                          desiredConfigRev: selectedDesired?.desiredConfigRev || deviceDetail.current?.configRev || '',
                          checkinIntervalSec: selectedDesired?.checkinIntervalSec ? String(selectedDesired.checkinIntervalSec) : '',
                        })
                      }}
                    >
                      Reset
                    </button>
                  </div>
                </form>

                <div className="inline-row">
                  <button className="button ghost" onClick={() => downloadLogs(selectedDeviceId)}>
                    Download Logs
                  </button>
                  <button
                    className="button ghost"
                    onClick={() => handleDeleteDevice(selectedDeviceId)}
                  >
                    Delete Device
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {artifactModalOpen && (
        <div className="modal-backdrop" onClick={() => setArtifactModalOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Select Artifact</h3>
              <button className="button ghost" onClick={() => setArtifactModalOpen(false)}>
                Close
              </button>
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Name</th>
                    <th>Version</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {artifacts.map((a) => (
                    <tr key={a.artifactId}>
                      <td className="mono">{a.artifactId}</td>
                      <td>{a.name}</td>
                      <td>{a.version}</td>
                      <td>
                        <button
                          className="button ghost"
                          onClick={() => {
                            setDeviceForm({ ...deviceForm, artifactId: a.artifactId })
                            setDeviceFormDirty(true)
                            setArtifactModalOpen(false)
                          }}
                        >
                          Select
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
