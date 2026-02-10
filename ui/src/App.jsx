import { useEffect, useMemo, useRef, useState } from 'react'
import {
  getDevices,
  getDevice,
  listGroups,
  putGroup,
  deleteGroup,
  patchDevice,
  clearDesiredStateDevice,
  getDesiredState,
  listArtifacts,
  uploadArtifact,
  setDesiredStateDevice,
  setDesiredStateGroup,
  clearDesiredStateGroup,
  getDeviceLogs,
  deleteDevice,
  deleteArtifact,
  getMaintenance,
  setMaintenance,
  getUpgradeStatus,
  getUpgradeAvailable,
  getUpgradePreflight,
  applyUpgrade,
  listAuditEvents,
  downloadAuditCSV,
  getAuditRetention,
  setAuditRetention,
  login as apiLogin,
  getMe,
  getAuthStatus,
  registerWithVoucher,
  createVoucher,
  listUsers,
  createUser,
  getAuthToken,
  setAuthToken as persistAuthToken,
} from './api'

const nav = [
  { id: 'dashboard', label: 'Dashboard', icon: 'icon-dashboard' },
  { id: 'logs', label: 'Logs', icon: 'icon-logs' },
  { id: 'settings', label: 'Settings', icon: 'icon-settings' },
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

function formatSelector(selector) {
  const entries = Object.entries(selector || {})
  if (entries.length === 0) return '—'
  return entries.map(([key, value]) => `${key}=${value}`).join(', ')
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

export default function App() {
  const apiBaseUrl = useMemo(() => {
    return import.meta.env.VITE_API_BASE_URL || 'https://localhost:8080'
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
  const [authError, setAuthError] = useState('')
  const [loginForm, setLoginForm] = useState({ email: '', password: '' })
  const [loginStatus, setLoginStatus] = useState('')
  const [authView, setAuthView] = useState('login')
  const [authStatus, setAuthStatus] = useState({ enabled: false, mode: 'disabled', loaded: false })
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

  useEffect(() => {
    const onHash = () => {
      const hash = window.location.hash.replace('#', '')
      setView(hash || 'dashboard')
    }
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
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
    async function loadMe() {
      if (!authToken) {
        setAuthUser(null)
        return
      }
      setAuthError('')
      try {
        const res = await getMe()
        setAuthUser(res)
      } catch (err) {
        persistAuthToken('')
        setAuthToken('')
        setAuthUser(null)
        setAuthError(err.message || String(err))
      }
    }
    loadMe()
  }, [authToken])

  useEffect(() => {
    if (!authToken) return
    loadUsers()
  }, [authToken])

  const [devices, setDevices] = useState([])
  const [devicesLoading, setDevicesLoading] = useState(false)
  const [devicesError, setDevicesError] = useState('')
  const [devicesStatus, setDevicesStatus] = useState('')
  const [deviceOrder, setDeviceOrder] = useState([])
  const [deviceStatusFilter, setDeviceStatusFilter] = useState('all')

  const [selectedDeviceId, setSelectedDeviceId] = useState('')
  const [deviceDetail, setDeviceDetail] = useState(null)
  const [deviceDetailError, setDeviceDetailError] = useState('')
  const [deviceDrawerOpen, setDeviceDrawerOpen] = useState(false)
  const [artifactModalOpen, setArtifactModalOpen] = useState(false)
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
    artifactId: '',
    desiredVersion: '',
    desiredConfigRev: '',
    checkinIntervalSec: '',
  })

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
  const [logsTab, setLogsTab] = useState('device')

  const [eventsFeed, setEventsFeed] = useState([])
  const [eventsStatus, setEventsStatus] = useState('disconnected')
  const [eventsError, setEventsError] = useState('')
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
  const selectedDeviceIdRef = useRef('')
  const refreshTimerRef = useRef(null)
  const deviceOrderRef = useRef([])
  const eventsConnRef = useRef('disconnected')
  const lastEventAtRef = useRef(0)
  const eventsHeartbeatRef = useRef(null)

  const [theme, setTheme] = useState(() => {
    return localStorage.getItem('hwops-theme') || 'dark'
  })
  const maintenanceToken = import.meta.env.VITE_MAINTENANCE_TOKEN || ''
  const canToggleMaintenance = Boolean(maintenanceToken)

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('hwops-theme', theme)
  }, [theme])

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
    loadDevices()
    loadGroups()
    loadArtifacts()
    loadDesired()
    loadMaintenance()
    loadUpgrade()
    loadUpgradeAvailable()
    loadUpgradePreflight()
    loadAuditRetention()
  }, [authStatus.loaded, authStatus.enabled, authToken])

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
    if (!selectedGroupId) return
    const desired = desiredState.groups?.find((g) => g.groupId === selectedGroupId)
    setGroupDesiredForm({
      groupId: selectedGroupId,
      artifactId: desired?.artifactId || '',
      desiredVersion: desired?.desiredVersion || '',
      desiredConfigRev: desired?.desiredConfigRev || '',
      checkinIntervalSec: desired?.checkinIntervalSec ? String(desired.checkinIntervalSec) : '',
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
          setEventsFeed((prev) => [data, ...prev].slice(0, 200))
          lastEventAtRef.current = Date.now()
          if (!refreshTimerRef.current) {
            refreshTimerRef.current = setTimeout(() => {
              refreshTimerRef.current = null
              loadDevices({ silent: true })
              const currentId = selectedDeviceIdRef.current
              if (currentId) {
                getDevice(currentId)
                  .then(setDeviceDetail)
                  .catch((err) => setDeviceDetailError(err.message || String(err)))
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
    const total = devices.length
    const active = devices.filter((d) => d.status === 'active').length
    const lastSeen = devices
      .map((d) => (d.lastSeen ? new Date(d.lastSeen) : null))
      .filter(Boolean)
      .sort((a, b) => b - a)[0]
    const lastSeenLabel = lastSeen ? lastSeen.toISOString() : '—'
    return { total, active, lastSeenLabel }
  }, [devices, deviceDetail])

  const preApplyBadgeClass = useMemo(() => {
    const raw = (deviceDetail?.current?.lastPreApplyStatus || '').toLowerCase()
    if (!raw || raw === '—') return 'unknown'
    return raw
  }, [deviceDetail])

  const isAdmin = useMemo(() => {
    return (authUser?.roles || []).includes('admin')
  }, [authUser])

  const notifications = useMemo(() => {
    const items = []
    if (devicesError) items.push({ type: 'error', text: devicesError })
    if (groupsError) items.push({ type: 'error', text: groupsError })
    if (artifactsError) items.push({ type: 'error', text: artifactsError })
    if (desiredError) items.push({ type: 'error', text: desiredError })
    if (eventsError) items.push({ type: 'error', text: eventsError })
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
    return items.slice(0, 4)
  }, [devicesError, groupsError, artifactsError, desiredError, eventsError, maintenanceError, upgradeError, upgradeAvailableError, uploadStatus, devicesStatus, groupsStatus, artifactsStatus, desiredStatus, maintenanceStatus, upgradeStatus])

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
      setRegisterForm({ token: '', email: '', password: '', displayName: '' })
      setRegisterStatus('')
      setAuthView('login')
    } catch (err) {
      setRegisterStatus('')
      setAuthError(err.message || String(err))
    }
  }

  function doLogout() {
    persistAuthToken('')
    setAuthToken('')
    setAuthUser(null)
  }

  async function loadUsers() {
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
        setArtifactUploadOpen(false)
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

  async function handleSaveGroup(e) {
    e.preventDefault()
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

  async function handleDeleteGroup(groupId) {
    const ok = window.confirm(`Delete group ${groupId}? This removes desired state for the group.`)
    if (!ok) return
    setGroupsStatus('Deleting group...')
    try {
      await deleteGroup(groupId)
      setGroupsStatus(`Deleted group ${groupId}`)
      if (selectedGroupId === groupId) {
        setSelectedGroupId('')
      }
      loadGroups()
      loadDesired()
    } catch (err) {
      setGroupsError(err.message || String(err))
    }
  }

  async function handleGroupDeviceToggle(group, device, shouldAdd) {
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
    setGroupsStatus('Setting group desired state...')
    const payload = {
      artifactId: groupDesiredForm.artifactId || undefined,
      desiredVersion: groupDesiredForm.desiredVersion || undefined,
      desiredConfigRev: groupDesiredForm.desiredConfigRev || undefined,
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

  async function handleClearGroupDesired() {
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

  async function downloadAudit() {
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

  async function loadAuditRetention() {
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

  async function updateAuditRetention() {
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

  async function toggleMaintenance() {
    if (!canToggleMaintenance) return
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
      const res = await setMaintenance({ enabled: nextEnabled, message }, maintenanceToken)
      setMaintenanceState(res)
      setMaintenanceStatus(nextEnabled ? 'Maintenance enabled' : 'Maintenance disabled')
    } catch (err) {
      setMaintenanceStatus(err.message || String(err))
    }
  }

  async function startUpgrade() {
    if (!canToggleMaintenance || !upgrade.enabled) return
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
      const res = await applyUpgrade(maintenanceToken)
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
  const selectedArtifact = artifacts.find((a) => a.artifactId === deviceForm.artifactId)
  const selectedGroupArtifact = artifacts.find((a) => a.artifactId === groupDesiredForm.artifactId)
  const lastAppliedArtifact = artifacts.find((a) => a.artifactId === deviceDetail?.current?.lastApplyArtifactId)
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
  const selectedArtifactGroup = selectedArtifact
    ? artifactGroups.find((g) => g.name === selectedArtifact.name)
    : null
  const selectedGroupArtifactGroup = selectedGroupArtifact
    ? artifactGroups.find((g) => g.name === selectedGroupArtifact.name)
    : null
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
  const filteredDevices = useMemo(() => {
    if (deviceStatusFilter === 'all') return devices
    return devices.filter((d) => (d.status || '').toLowerCase() === deviceStatusFilter)
  }, [devices, deviceStatusFilter])

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
        </div>
      </div>
    )
  }

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
                      {filteredDevices.map((d) => (
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
                          <td colSpan={4}>No devices match this filter.</td>
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
                  <button
                    onClick={() => {
                      setGroupForm({ groupId: '', name: '', region: '', role: '', site: '', custom: [] })
                      setGroupModalOpen(true)
                    }}
                    className="button"
                  >
                    Add Group
                  </button>
                  <button onClick={loadGroups} className="button ghost">Refresh</button>
                </div>
              </div>
              {groupsError && <div className="error">{groupsError}</div>}

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Selector</th>
                      <th>Devices</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {groups.map((group) => (
                      <tr key={group.groupId} className={selectedGroupId === group.groupId ? 'selected' : ''}>
                        <td>{group.name || group.groupId}</td>
                        <td><code>{formatSelector(group.selector || {})}</code></td>
                        <td>{groupCounts[group.groupId] ?? 0}</td>
                        <td>
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
                          <button
                            className="button ghost"
                            onClick={() => setSelectedGroupId(group.groupId)}
                          >
                            Manage Devices
                          </button>
                          <button
                            className="button ghost"
                            onClick={() => {
                              setSelectedGroupId(group.groupId)
                              setGroupDesiredOpen(true)
                            }}
                          >
                            Set Desired
                          </button>
                          <button
                            className="button ghost"
                            onClick={() => handleDeleteGroup(group.groupId)}
                          >
                            Delete
                          </button>
                        </td>
                      </tr>
                    ))}
                    {groups.length === 0 && (
                      <tr>
                        <td colSpan={4}>No groups yet.</td>
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
                              <button
                                className="button ghost"
                                onClick={() => handleGroupDeviceToggle(selectedGroup, item.device, !item.inGroup)}
                                disabled={Object.keys(selectedGroupSelector).length === 0}
                              >
                                {item.inGroup ? 'Remove' : 'Add'}
                              </button>
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
                    <div className="hint">Empty selector matches all devices. Add keys to enable membership control.</div>
                  )}
                </div>
              )}
            </section>

            <section id="artifacts" className="card">
          <div className="section-header">
            <h2>Artifacts</h2>
            <div className="inline-row">
              <button onClick={() => setArtifactUploadOpen(true)} className="button">Upload</button>
              <button onClick={loadArtifacts} className="button ghost">Refresh</button>
            </div>
          </div>
          {artifactsError && <div className="error">{artifactsError}</div>}

          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Artifact ID</th>
                  <th>Version</th>
                  <th>Signed</th>
                  <th>SHA256</th>
                  <th>Size</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {artifactGroups.map((group) => (
                  group.versions.map((a, idx) => (
                    <tr key={a.artifactId} className={idx === 0 ? 'artifact-group-start' : ''}>
                      <td>{idx === 0 ? group.name : ''}</td>
                      <td>{a.artifactId}</td>
                      <td>{a.version}</td>
                      <td>
                        <span className={`pill ${a.signature ? 'signed' : 'unsigned'}`}>
                          {a.signature ? 'signed' : 'unsigned'}
                        </span>
                      </td>
                      <td className="mono">{a.sha256}</td>
                      <td>{a.sizeBytes}</td>
                      <td>{a.createdAt}</td>
                      <td>
                        <button className="button ghost" onClick={() => handleDeleteArtifact(a.artifactId)}>
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))
                ))}
                {artifactGroups.length === 0 && (
                  <tr>
                    <td colSpan={8}>No artifacts uploaded yet.</td>
                  </tr>
                )}
              </tbody>
              </table>
            </div>
            </section>
          </>
        )}

        {view === 'logs' && (
          <section id="logs" className="card logs-card">
            <div className="section-header logs-header">
              <div className="tab-bar">
                <button
                  className={`tab ${logsTab === 'device' ? 'active' : ''}`}
                  onClick={() => setLogsTab('device')}
                >
                  Device Logs
                </button>
                <button
                  className={`tab ${logsTab === 'audit' ? 'active' : ''}`}
                  onClick={() => setLogsTab('audit')}
                >
                  Audit Log
                </button>
              </div>
              <div className="logs-actions">
                {logsTab === 'device' ? (
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
                ) : (
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

            {logsTab === 'device' ? (
              <>
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
              </>
            ) : (
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

        {view === 'settings' && (
          <section id="settings" className="card settings-card">
            <div className="section-header settings-header">
              <h2>Settings</h2>
              <div className="settings-toolbar">
                <button className="button ghost" onClick={loadMaintenance}>Refresh maintenance</button>
                <button className="button ghost" onClick={loadUpgrade}>Refresh upgrade</button>
                <button className="button ghost" onClick={loadUpgradeAvailable}>Refresh updates</button>
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
                ) : !isAdmin ? (
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
                    <button className="button ghost" onClick={toggleMaintenance} disabled={!canToggleMaintenance}>
                      {maintenance.enabled ? 'Disable maintenance' : 'Enable maintenance'}
                    </button>
                  </div>
                </div>
              </div>

              <div className="settings-section">
              <div className="settings-title">Update Packages</div>
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
                {canToggleMaintenance && maintenance.enabled && upgrade.enabled && (
                  <div className="inline-row">
                    <button
                      className="button ghost"
                      onClick={startUpgrade}
                      disabled={upgrade.running || !upgradeAvailable.available}
                    >
                      {upgrade.running ? 'Applying update…' : 'Apply update'}
                    </button>
                  </div>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-title">Upgrade Preflight</div>
                <div className="inline-row">
                  <button className="button ghost" onClick={loadUpgradePreflight}>
                    Run preflight
                  </button>
                  {upgradePreflightStatus && <div className="status">{upgradePreflightStatus}</div>}
                </div>
                {upgradePreflightError && <div className="error">{upgradePreflightError}</div>}
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
              </div>

              <div className="settings-section">
                <div className="settings-title">Upgrade Status</div>
                {!upgrade.enabled ? (
                  <div className="placeholder">Upgrade runner not configured.</div>
                ) : (
                  <div className="detail-grid">
                    <div>
                      <div className="detail-label">State</div>
                      <div className="detail-value">{upgrade.state || 'idle'}</div>
                    </div>
                    <div>
                      <div className="detail-label">Running</div>
                      <div className="detail-value">{upgrade.running ? 'yes' : 'no'}</div>
                    </div>
                    <div>
                      <div className="detail-label">Exit Code</div>
                      <div className="detail-value">{upgrade.exitCode ?? '—'}</div>
                    </div>
                    <div className="full">
                      <div className="detail-label">Runner Container</div>
                      <div className="detail-value">{upgrade.runnerContainer || '—'}</div>
                    </div>
                    <div className="full">
                      <div className="detail-label">Last Started</div>
                      <div className="detail-value">{upgrade.startedAt ? new Date(upgrade.startedAt).toLocaleString() : '—'}</div>
                    </div>
                    <div className="full">
                      <div className="detail-label">Last Finished</div>
                      <div className="detail-value">{upgrade.finishedAt ? new Date(upgrade.finishedAt).toLocaleString() : '—'}</div>
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
                )}
              </div>
            </div>
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
                    <select
                      value={selectedArtifact?.name || ''}
                      onChange={(e) => {
                        const name = e.target.value
                        const group = artifactGroups.find((g) => g.name === name)
                        if (!group) {
                          setDeviceForm({ ...deviceForm, artifactId: '', desiredVersion: '' })
                        } else {
                          const pick = group.versions[group.versions.length - 1]
                          setDeviceForm({
                            ...deviceForm,
                            artifactId: pick.artifactId,
                            desiredVersion: pick.version,
                          })
                        }
                        setDeviceFormDirty(true)
                      }}
                    >
                      <option value="">Select artifact</option>
                      {artifactGroups.map((group) => (
                        <option key={group.name} value={group.name}>{group.name}</option>
                      ))}
                    </select>
                    <button className="button ghost" type="button" onClick={() => setArtifactModalOpen(true)}>
                      Browse
                    </button>
                  </div>
                  <label>Artifact Version</label>
                  <select
                    value={selectedArtifact?.version || ''}
                    onChange={(e) => {
                      const version = e.target.value
                      const group = selectedArtifactGroup
                      const pick = group?.versions.find((v) => v.version === version)
                      if (pick) {
                        setDeviceForm({
                          ...deviceForm,
                          artifactId: pick.artifactId,
                          desiredVersion: pick.version,
                        })
                        setDeviceFormDirty(true)
                      }
                    }}
                  >
                    <option value="">Select version</option>
                    {(selectedArtifactGroup?.versions || []).map((artifact) => (
                      <option key={artifact.artifactId} value={artifact.version}>
                        {artifact.version}
                      </option>
                    ))}
                  </select>
                  <label>Artifact ID</label>
                  <input value={deviceForm.artifactId} readOnly placeholder="artifact uuid" />
                  {selectedArtifact && (
                    <div className="artifact-summary">
                      <div><strong>{selectedArtifact.name}</strong> v{selectedArtifact.version}</div>
                      <div className="mono">{selectedArtifact.artifactId}</div>
                      <div>
                        <span className={`pill ${selectedArtifact.signature ? 'signed' : 'unsigned'}`}>
                          {selectedArtifact.signature ? 'signed' : 'unsigned'}
                        </span>
                      </div>
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
                    <th>Name</th>
                    <th>ID</th>
                    <th>Version</th>
                    <th>Signed</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {artifactGroups.map((group) => (
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
                              setDeviceForm({ ...deviceForm, artifactId: a.artifactId, desiredVersion: a.version })
                              setDeviceFormDirty(true)
                              setArtifactModalOpen(false)
                            }}
                          >
                            Select
                          </button>
                        </td>
                      </tr>
                    ))
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {artifactUploadOpen && (
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

      {groupModalOpen && (
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

      {groupDesiredOpen && (
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
              <label>Artifact</label>
              <div className="inline-row">
                <select
                  value={selectedGroupArtifact?.name || ''}
                  onChange={(e) => {
                    const name = e.target.value
                    const group = artifactGroups.find((g) => g.name === name)
                    if (!group) {
                      setGroupDesiredForm({ ...groupDesiredForm, artifactId: '', desiredVersion: '' })
                    } else {
                      const pick = group.versions[group.versions.length - 1]
                      setGroupDesiredForm({
                        ...groupDesiredForm,
                        artifactId: pick.artifactId,
                        desiredVersion: pick.version,
                      })
                    }
                  }}
                >
                  <option value="">Select artifact</option>
                  {artifactGroups.map((group) => (
                    <option key={group.name} value={group.name}>{group.name}</option>
                  ))}
                </select>
                <button className="button ghost" type="button" onClick={() => setArtifactModalOpen(true)}>
                  Browse
                </button>
              </div>
              <label>Artifact Version</label>
              <select
                value={selectedGroupArtifact?.version || ''}
                onChange={(e) => {
                  const version = e.target.value
                  const group = selectedGroupArtifactGroup
                  const pick = group?.versions.find((v) => v.version === version)
                  if (pick) {
                    setGroupDesiredForm({
                      ...groupDesiredForm,
                      artifactId: pick.artifactId,
                      desiredVersion: pick.version,
                    })
                  }
                }}
              >
                <option value="">Select version</option>
                {(selectedGroupArtifactGroup?.versions || []).map((artifact) => (
                  <option key={artifact.artifactId} value={artifact.version}>
                    {artifact.version}
                  </option>
                ))}
              </select>
              <label>Artifact ID</label>
              <input value={groupDesiredForm.artifactId} readOnly placeholder="artifact uuid" />
              {selectedGroupArtifact && (
                <div className="artifact-summary">
                  <div><strong>{selectedGroupArtifact.name}</strong> v{selectedGroupArtifact.version}</div>
                  <div className="mono">{selectedGroupArtifact.artifactId}</div>
                  <div>
                    <span className={`pill ${selectedGroupArtifact.signature ? 'signed' : 'unsigned'}`}>
                      {selectedGroupArtifact.signature ? 'signed' : 'unsigned'}
                    </span>
                  </div>
                </div>
              )}
              <label>Desired Version</label>
              <input
                value={groupDesiredForm.desiredVersion}
                onChange={(e) => setGroupDesiredForm({ ...groupDesiredForm, desiredVersion: e.target.value })}
                placeholder="1.0.0"
              />
              <label>Config Rev</label>
              <input
                value={groupDesiredForm.desiredConfigRev}
                onChange={(e) => setGroupDesiredForm({ ...groupDesiredForm, desiredConfigRev: e.target.value })}
                placeholder="c1"
              />
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
    </div>
  )
}
