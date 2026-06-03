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
  listTrustedSigningKeys,
  createTrustedSigningKey,
  updateTrustedSigningKey,
  retireTrustedSigningKey,
  getArtifactTrustPolicy,
  setArtifactTrustPolicy as saveArtifactTrustPolicy,
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
  ldapLogin as apiLdapLogin,
  generateRecoveryCodes as apiGenerateRecoveryCodes,
  resetPasswordWithRecoveryCode as apiResetPasswordWithRecoveryCode,
  createPasswordResetToken as apiCreatePasswordResetToken,
  completePasswordResetToken as apiCompletePasswordResetToken,
  getMe,
  getAuthStatus,
  getBootstrapStatus,
  downloadBootstrapCA,
  registerWithVoucher,
  createVoucher,
  listUsers,
  createUser,
  listArtifactVulnScans,
  getLatestArtifactVulnScan,
  triggerArtifactScan,
  getLatestDeviceVulnScan,
  listDeviceVulnScans,
  triggerNessusSync,
  getNessusSyncStatus,
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
  forgotPassword as apiForgotPassword,
  sendUserInvite as apiSendUserInvite,
} from './api'
import { buildPermissionState, firstAllowedKey, hasRole } from './rbac'
import { variant } from '@variant'
const { brandName, allowedViews, extraViews, themeOverrides, featureFlags } = variant
import ArtifactPickerModal from './components/modals/ArtifactPickerModal'
import ArtifactUploadModal from './components/modals/ArtifactUploadModal'
import TrustOverrideModal from './components/modals/TrustOverrideModal'
import GroupModal from './components/modals/GroupModal'
import TrustedSigningKeyModal from './components/modals/TrustedSigningKeyModal'
import EnrollmentProfileCreateModal from './components/modals/EnrollmentProfileCreateModal'
import EnrollmentProfileEditModal from './components/modals/EnrollmentProfileEditModal'
import SecurityPage from './features/security/SecurityPage'
import SettingsPage from './features/settings/SettingsPage'
import GlobalPage from './features/global/GlobalPage'
import DeviceDrawer from './features/devices/DeviceDrawer'
import DevicesSection from './features/devices/DevicesSection'
import GroupMultiDesiredModal from './features/groups/GroupMultiDesiredModal'
import GroupDesiredModal from './features/groups/GroupDesiredModal'
import BulkGroupManagementModal from './features/groups/BulkGroupManagementModal'
import GroupsSection from './features/groups/GroupsSection'

import {
  nav,
  logsNav,
  componentTypes,
  rangeOptions,
  rangeMsById,
  chartColors,
  groupTrackingModes,
  deviceTrackingModes,
  trustOverrideModes,
  signatureTypeOptions,
  parsePolicyObject,
  getAutoTrackModeFromPolicy,
  getTrackingPolicyFromPolicy,
  mergeTrackingIntoPolicy,
  verificationModeLabel,
  trustPolicyStrictness,
  normalizeTrustOverride,
  mergeTrustOverrideIntoPolicy,
  resolveEffectiveTrustPolicy,
  hasTrustOverride,
  trustPolicySummary,
  formatChartValue,
  TimeSeriesChart,
  ChartLegend,
  newComponentRow,
  normalizeArtifactType,
  normalizeArtifactStatus,
  parseSemver4,
  compareSemver4,
  normalizeVerificationStatus,
  verificationPillLabel,
  artifactSignerSummary,
  artifactAllowedByTrustPolicy,
  filterArtifactGroupsByType,
  filterArtifactGroupsByTrustPolicy,
  fallbackComponentType,
  parseCSVLine,
  parseCSV,
  normalizeObject,
  selectorMatches,
  selectorToForm,
  objectToKeyValueRows,
  keyValueRowsToObject,
  formatSelector,
  normalizeSelector,
  stableSelectorString,
  isUUID,
  csvEscape,
  buildBulkGroupsRollbackCsv,
  parseBulkGroupsCsv,
  toIsoIfValid,
  formatTime,
  formatNumber,
  formatBytes,
  formatDurationSeconds,
  downloadTextFile,
  formatObjectSummary,
  parsePrometheusMetrics,
  sumLabeled,
  groupLabeledMetrics,
} from './lib/appShared'

const navIcons = {
  'icon-dashboard': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <rect x="1" y="1" width="6" height="6" rx="1.5"/>
      <rect x="9" y="1" width="6" height="6" rx="1.5"/>
      <rect x="1" y="9" width="6" height="6" rx="1.5"/>
      <rect x="9" y="9" width="6" height="6" rx="1.5"/>
    </svg>
  ),
  'icon-metrics': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <rect x="1" y="9" width="3" height="6" rx="1"/>
      <rect x="6" y="5" width="3" height="10" rx="1"/>
      <rect x="11" y="2" width="3" height="13" rx="1"/>
    </svg>
  ),
  'icon-logs': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
      <line x1="2" y1="4" x2="14" y2="4"/>
      <line x1="2" y1="8" x2="14" y2="8"/>
      <line x1="2" y1="12" x2="10" y2="12"/>
    </svg>
  ),
  'icon-security': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 1L2 3.5V8c0 3.3 2.5 5.8 6 6.9C11.5 13.8 14 11.3 14 8V3.5L8 1z"/>
    </svg>
  ),
  'icon-settings': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true">
      <line x1="2" y1="4" x2="14" y2="4"/>
      <line x1="2" y1="8" x2="14" y2="8"/>
      <line x1="2" y1="12" x2="14" y2="12"/>
      <circle cx="10" cy="4" r="1.5" fill="currentColor" stroke="none"/>
      <circle cx="5" cy="8" r="1.5" fill="currentColor" stroke="none"/>
      <circle cx="11" cy="12" r="1.5" fill="currentColor" stroke="none"/>
    </svg>
  ),
  'icon-global': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true">
      <circle cx="8" cy="8" r="6.5"/>
      <path d="M8 1.5C6.2 3.5 5 5.6 5 8s1.2 4.5 3 6.5"/>
      <path d="M8 1.5c1.8 2 3 4.1 3 6.5s-1.2 4.5-3 6.5"/>
      <line x1="1.5" y1="8" x2="14.5" y2="8"/>
    </svg>
  ),
  'icon-compliance': (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 1L2 3.5V8c0 3.3 2.5 5.8 6 6.9C11.5 13.8 14 11.3 14 8V3.5L8 1z"/>
      <polyline points="5,8 7,10.5 11,6" fill="none" stroke="white" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  ),
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
  const [ldapForm, setLdapForm] = useState({ username: '', password: '' })
  const [ldapStatus, setLdapStatus] = useState('')
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
  const [recoveryForm, setRecoveryForm] = useState({
    email: '',
    recoveryCode: '',
    newPassword: '',
  })
  const [recoveryStatus, setRecoveryStatus] = useState('')
  const [resetTokenForm, setResetTokenForm] = useState({
    email: '',
    resetToken: '',
    newPassword: '',
  })
  const [resetTokenStatus, setResetTokenStatus] = useState('')
  const [forgotPasswordForm, setForgotPasswordForm] = useState({ email: '' })
  const [forgotPasswordStatus, setForgotPasswordStatus] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState([])
  const [recoveryCodesGeneratedAt, setRecoveryCodesGeneratedAt] = useState('')
  const [recoveryCodesStatus, setRecoveryCodesStatus] = useState('')
  const [recoveryCodesError, setRecoveryCodesError] = useState('')
  const [users, setUsers] = useState([])
  const [usersError, setUsersError] = useState('')
  const [usersStatus, setUsersStatus] = useState('')
  const [userForm, setUserForm] = useState({
    email: '',
    password: '',
    displayName: '',
    roles: ['viewer'],
  })
  const [passwordResetIssueForm, setPasswordResetIssueForm] = useState({
    ttlMinutes: '15',
    reason: '',
  })
  const [passwordResetTokenValue, setPasswordResetTokenValue] = useState('')
  const [passwordResetTokenTarget, setPasswordResetTokenTarget] = useState(null)
  const [passwordResetTokenStatus, setPasswordResetTokenStatus] = useState('')
  const [passwordResetTokenError, setPasswordResetTokenError] = useState('')
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
  const coreNav = useMemo(
    () => nav.filter((item) => allowedViews.includes(item.id) && permissions.views[item.id]),
    [permissions],
  )
  const extraNavVisible = useMemo(
    () => extraViews.filter((item) =>
      !item.requiredRole || hasRole(authUser?.roles || [], item.requiredRole)
    ),
    [authUser],
  )
  const visibleNav = useMemo(
    () => [...coreNav, ...extraNavVisible],
    [coreNav, extraNavVisible],
  )
  const visibleLogsTabs = useMemo(
    () => logsNav.filter((item) => permissions.logsTabs[item.id]),
    [permissions],
  )
  const extraViewPerms = useMemo(
    () => Object.fromEntries(
      extraViews.map(({ id, requiredRole }) => [
        id, hasRole(authUser?.roles || [], requiredRole || 'viewer'),
      ])
    ),
    [authUser],
  )
  const allViewPerms = useMemo(
    () => ({ ...permissions.views, ...extraViewPerms }),
    [permissions.views, extraViewPerms],
  )
  const fallbackView = useMemo(
    () => firstAllowedKey(allViewPerms, 'dashboard'),
    [allViewPerms],
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
    // Pre-fill the reset-token form from URL params (e.g. email links from ForgotPassword / SendUserInvite).
    const params = new URLSearchParams(window.location.search)
    const view = params.get('view')
    const email = params.get('email')
    const token = params.get('token')
    if (view === 'reset-token' && (email || token)) {
      setAuthView('reset-token')
      setResetTokenForm((prev) => ({
        ...prev,
        email: email || prev.email,
        resetToken: token || prev.resetToken,
      }))
      // Clean up URL params without reloading.
      params.delete('view')
      params.delete('email')
      params.delete('token')
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
    if (!authStatus.loaded || allViewPerms[view]) return
    setView(fallbackView)
    if (window.location.hash.replace('#', '') !== fallbackView) {
      window.location.hash = fallbackView
    }
  }, [authStatus.loaded, allViewPerms, view, fallbackView])

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
  const [artifactTrackingDetailKey, setArtifactTrackingDetailKey] = useState('')
  const [artifactsError, setArtifactsError] = useState('')
  // artifactVulnScans: { [artifactId]: scan | null | 'loading' }
  const [artifactVulnScans, setArtifactVulnScans] = useState({})
  const [artifactVulnPanelId, setArtifactVulnPanelId] = useState('')
  // nessusSyncStatus: { lastSyncAt, matchCount, error }
  const [nessusSyncStatus, setNessusSyncStatus] = useState(null)
  const [nessusSyncing, setNessusSyncing] = useState(false)
  const [artifactsStatus, setArtifactsStatus] = useState('')
  const artifactByID = useMemo(() => {
    const map = {}
    artifacts.forEach((artifact) => {
      map[artifact.artifactId] = artifact
    })
    return map
  }, [artifacts])
  const [artifactTrustPolicy, setArtifactTrustPolicyState] = useState({
    verificationMode: 'warn_unsigned',
    allowedSigningKeyIds: [],
    allowedSignatureTypes: [],
    updatedAt: '',
    updatedByUserId: '',
  })
  const [artifactTrustStatus, setArtifactTrustStatus] = useState('')
  const [artifactTrustError, setArtifactTrustError] = useState('')
  const [trustedSigningKeys, setTrustedSigningKeys] = useState([])
  const [trustedSigningKeysLoading, setTrustedSigningKeysLoading] = useState(false)
  const [trustedSigningKeysLoaded, setTrustedSigningKeysLoaded] = useState(false)
  const [trustedSigningKeyModalOpen, setTrustedSigningKeyModalOpen] = useState(false)
  const [editingTrustedSigningKeyId, setEditingTrustedSigningKeyId] = useState('')
  const [trustedSigningKeyForm, setTrustedSigningKeyForm] = useState({
    displayName: '',
    algorithm: 'ed25519',
    publicKeyPem: '',
    notes: '',
  })
  const [trustOverrideEditor, setTrustOverrideEditor] = useState(null)
  const [trustOverrideForm, setTrustOverrideForm] = useState({
    verificationMode: 'inherit',
    allowedSigningKeyIds: [],
    allowedSignatureTypes: [],
  })
  const [trustOverrideError, setTrustOverrideError] = useState('')
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
  const [globalPlaneUrl, setGlobalPlaneUrl] = useState(() => {
    return (
      import.meta.env.VITE_GLOBAL_PLANE_URL ||
      (typeof localStorage !== 'undefined' && localStorage.getItem('hwops_global_plane_url')) ||
      ''
    )
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
        const tracking = getTrackingPolicyFromPolicy(policy)
        const selectedArtifact = comp.artifactId ? artifactByID[comp.artifactId] : null
        rows.push(newComponentRow({
          key,
          artifactType: tracking.artifactType || fallbackComponentType(key, comp.artifactType || selectedArtifact?.type),
          artifactId: comp.artifactId || '',
          desiredVersion: comp.desiredVersion || '',
          desiredConfigRev: comp.desiredConfigRev || '',
          policy,
          autoTrackMode: getAutoTrackModeFromPolicy(policy),
          trackingName: tracking.name || selectedArtifact?.name || '',
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
    return buildComponentsPayloadForScope(rows, 'group')
  }

  function buildComponentsPayloadForScope(rows, scope) {
    const components = {}
    for (const row of rows || []) {
      const mode = String(row.autoTrackMode || 'inherit').trim().toLowerCase()
      if (scope === 'device' && mode === 'inherit') {
        continue
      }
      const trackingRequiresTarget = scope === 'device' ? mode === 'enabled' : mode !== 'disabled'
      const hasValues = Boolean(
        row.key ||
        row.artifactType ||
        row.artifactId ||
        row.desiredVersion ||
        row.desiredConfigRev ||
        trackingRequiresTarget ||
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
      if (trackingRequiresTarget && !String(row.trackingName || '').trim()) {
        return { error: `Component ${key} requires a tracking artifact name.` }
      }
      const mergedPolicy = mergeTrackingIntoPolicy(row.policy, {
        mode,
        name: row.trackingName,
        artifactType,
      })
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

  function getTrustEditorRow(target = trustOverrideEditor) {
    if (!target) return null
    const { scope, index } = target
    if (scope === 'group') return groupDesiredForm.components?.[index] || null
    if (scope === 'groupBulk') return groupMultiDesiredForm.components?.[index] || null
    if (scope === 'device') return deviceForm.components?.[index] || null
    return null
  }

  function openTrustOverrideEditor(scope, index) {
    if (!canManageDesiredState) return
    const target = { scope, index }
    const row = getTrustEditorRow(target)
    if (!row) return
    setTrustOverrideForm(normalizeTrustOverride(row.policy))
    setTrustOverrideError('')
    setTrustOverrideEditor(target)
  }

  function closeTrustOverrideEditor() {
    setTrustOverrideEditor(null)
    setTrustOverrideError('')
  }

  function saveTrustOverrideEditor() {
    if (!trustOverrideEditor) return
    const globalMode = String(artifactTrustPolicy?.verificationMode || 'warn_unsigned')
    const requestedMode = String(trustOverrideForm.verificationMode || 'inherit')
    if (
      requestedMode !== 'inherit' &&
      trustPolicyStrictness(requestedMode) < trustPolicyStrictness(globalMode)
    ) {
      setTrustOverrideError(`Override cannot be weaker than the global policy (${verificationModeLabel(globalMode)}).`)
      return
    }
    const row = getTrustEditorRow()
    if (!row) return
    const nextPolicy = mergeTrustOverrideIntoPolicy(row.policy, trustOverrideForm)
    if (trustOverrideEditor.scope === 'group') {
      updateGroupComponent(trustOverrideEditor.index, { policy: nextPolicy })
    } else if (trustOverrideEditor.scope === 'groupBulk') {
      updateGroupMultiDesiredComponent(trustOverrideEditor.index, { policy: nextPolicy })
    } else if (trustOverrideEditor.scope === 'device') {
      updateDeviceComponent(trustOverrideEditor.index, { policy: nextPolicy })
    }
    closeTrustOverrideEditor()
  }

  function openArtifactPicker(scope, index) {
    if (!canManageDesiredState) return
    setArtifactPickerTarget({ scope, index })
    setArtifactModalOpen(true)
  }

  function shouldQueueTrackingRefresh(rows, scope) {
    return (rows || []).some((row) => {
      const mode = String(row?.autoTrackMode || 'inherit').trim().toLowerCase()
      if (scope === 'device') {
        return mode === 'enabled'
      }
      return mode !== 'disabled'
    })
  }

  function queueTrackingRefresh() {
    window.setTimeout(() => {
      loadDesired()
      loadArtifacts()
    }, 500)
    window.setTimeout(() => {
      loadDesired()
      loadArtifacts()
    }, 1500)
  }

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('hwops-theme', theme)
  }, [theme])

  useEffect(() => {
    const root = document.documentElement
    Object.entries(themeOverrides || {}).forEach(([k, v]) =>
      root.style.setProperty(
        `--${k.replace(/([A-Z])/g, '-$1').toLowerCase()}`,
        v,
      )
    )
  }, [])

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
    loadArtifactTrustPolicy({ silent: true }).catch(() => {})
    loadTrustedSigningKeys({ silent: true }).catch(() => {})
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
  }, [selectedDeviceId, desiredState, deviceDetail, deviceFormDirty, artifactByID])

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
  }, [selectedGroupId, desiredState, artifactByID])

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
  const canViewArtifactTrust = canManageArtifacts || canManageUsers
  const canManageArtifactTrust = canManageUsers
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
    if (canManageArtifactTrust) return
    setTrustedSigningKeyModalOpen(false)
    setEditingTrustedSigningKeyId('')
  }, [canManageArtifactTrust])

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
    setTrustOverrideEditor(null)
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
    loadArtifactTrustPolicy({ silent: true }).catch(() => {})
    loadTrustedSigningKeys({ silent: true }).catch(() => {})
    const timer = setInterval(() => {
      loadRotationStatus()
      loadEnrollmentProfiles({ silent: true })
      loadPendingEnrollments({ silent: true })
      loadArtifactTrustPolicy({ silent: true }).catch(() => {})
      loadTrustedSigningKeys({ silent: true }).catch(() => {})
    }, 15000)
    return () => clearInterval(timer)
  }, [view, canViewArtifactTrust])

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

  async function doLdapLogin() {
    setLdapStatus('Signing in...')
    setAuthError('')
    try {
      const res = await apiLdapLogin(ldapForm.username, ldapForm.password)
      if (!res?.token) {
        throw new Error('No token returned')
      }
      persistAuthToken(res.token)
      setAuthToken(res.token)
      setLdapStatus('Signed in')
      setLdapForm((prev) => ({ ...prev, password: '' }))
      setAuthUser(res.user || null)
      setAuthUserLoaded(true)
      setAuthView('login')
      loadUsers()
    } catch (err) {
      setLdapStatus('')
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

  async function doGenerateRecoveryCodes() {
    setRecoveryCodesStatus('Generating recovery codes...')
    setRecoveryCodesError('')
    try {
      const res = await apiGenerateRecoveryCodes()
      const codes = Array.isArray(res?.codes) ? res.codes : []
      setRecoveryCodes(codes)
      setRecoveryCodesGeneratedAt(res?.generatedAt || '')
      setRecoveryCodesStatus(codes.length > 0 ? 'Recovery codes generated. Store them now; they are only shown once.' : '')
      setAuthUser((prev) =>
        prev
          ? {
              ...prev,
              recoveryCodesConfigured: codes.length > 0,
              recoveryCodesGeneratedAt: res?.generatedAt || new Date().toISOString(),
            }
          : prev,
      )
      loadUsers()
    } catch (err) {
      setRecoveryCodesStatus('')
      setRecoveryCodesError(err.message || String(err))
    }
  }

  function doDownloadRecoveryCodes() {
    if (recoveryCodes.length === 0) return
    const lines = [
      'Parcel recovery codes',
      `Generated: ${recoveryCodesGeneratedAt ? formatTime(recoveryCodesGeneratedAt) : new Date().toLocaleString()}`,
      '',
      ...recoveryCodes,
      '',
      'Each code can be used once.',
    ]
    downloadTextFile('parcel-recovery-codes.txt', lines.join('\n'))
  }

  async function doResetWithRecoveryCode() {
    setRecoveryStatus('Resetting password...')
    setAuthError('')
    try {
      await apiResetPasswordWithRecoveryCode(
        recoveryForm.email,
        recoveryForm.recoveryCode,
        recoveryForm.newPassword,
      )
      setRecoveryStatus('Password reset. Sign in with your new password.')
      setLoginForm((prev) => ({ ...prev, email: recoveryForm.email, password: '' }))
      setRecoveryForm({ email: recoveryForm.email, recoveryCode: '', newPassword: '' })
      setAuthView('login')
    } catch (err) {
      setRecoveryStatus('')
      setAuthError(err.message || String(err))
    }
  }

  async function doResetWithPasswordResetToken() {
    setResetTokenStatus('Resetting password...')
    setAuthError('')
    try {
      await apiCompletePasswordResetToken(
        resetTokenForm.email,
        resetTokenForm.resetToken,
        resetTokenForm.newPassword,
      )
      setResetTokenStatus('Password reset. Sign in with your new password.')
      setLoginForm((prev) => ({ ...prev, email: resetTokenForm.email, password: '' }))
      setResetTokenForm({ email: resetTokenForm.email, resetToken: '', newPassword: '' })
      setAuthView('login')
    } catch (err) {
      setResetTokenStatus('')
      setAuthError(err.message || String(err))
    }
  }

  async function doForgotPassword() {
    setForgotPasswordStatus('Sending...')
    const msg = 'If that address is registered, a reset link has been sent.'
    try {
      await apiForgotPassword(forgotPasswordForm.email)
      setForgotPasswordStatus(msg)
    } catch {
      // Always show the same message — no enumeration.
      setForgotPasswordStatus(msg)
    }
  }

  async function handleIssuePasswordResetToken(user) {
    setPasswordResetTokenStatus(`Issuing reset token for ${user.email}...`)
    setPasswordResetTokenError('')
    try {
      const ttlMinutes = Number.parseInt(passwordResetIssueForm.ttlMinutes, 10)
      const res = await apiCreatePasswordResetToken(user.userId, {
        ttlMinutes: Number.isFinite(ttlMinutes) ? ttlMinutes : 15,
        reason: passwordResetIssueForm.reason,
      })
      setPasswordResetTokenValue(res?.token || '')
      setPasswordResetTokenTarget({
        userId: user.userId,
        email: user.email,
        expiresAt: res?.expiresAt || '',
      })
      setPasswordResetTokenStatus(`Reset token issued for ${user.email}. Store it now; it is only shown once.`)
    } catch (err) {
      setPasswordResetTokenStatus('')
      setPasswordResetTokenError(err.message || String(err))
    }
  }

  async function handleSendUserInvite(user) {
    setPasswordResetTokenStatus(`Sending invite to ${user.email}...`)
    setPasswordResetTokenError('')
    try {
      await apiSendUserInvite(user.userId)
      setPasswordResetTokenStatus(`Invite sent to ${user.email}.`)
      setPasswordResetTokenValue('')
      setPasswordResetTokenTarget(null)
    } catch (err) {
      setPasswordResetTokenStatus('')
      setPasswordResetTokenError(err.message || String(err))
    }
  }

  async function doDownloadBootstrapCA() {
    setBootstrapStatusMessage('Downloading CA certificate...')
    try {
      const blob = await downloadBootstrapCA(bootstrapToken.trim() || undefined)
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'parcel-ca.crt'
      document.body.appendChild(a)
      a.click()
      a.remove()
      window.URL.revokeObjectURL(url)
      setBootstrapStatusMessage('Downloaded parcel-ca.crt')
    } catch (err) {
      setBootstrapStatusMessage(err.message || String(err))
    }
  }

  function doLogout() {
    persistAuthToken('')
    setAuthToken('')
    setAuthUser(null)
    setAuthUserLoaded(true)
    setRecoveryCodes([])
    setRecoveryCodesGeneratedAt('')
    setRecoveryCodesStatus('')
    setRecoveryCodesError('')
    setPasswordResetTokenValue('')
    setPasswordResetTokenTarget(null)
    setPasswordResetTokenStatus('')
    setPasswordResetTokenError('')
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

  function loadArtifactTrustPolicy({ silent = false } = {}) {
    if (!canViewArtifactTrust) {
      setArtifactTrustPolicyState({
        verificationMode: 'warn_unsigned',
        allowedSigningKeyIds: [],
        allowedSignatureTypes: [],
        updatedAt: '',
        updatedByUserId: '',
      })
      return Promise.resolve(null)
    }
    if (!silent) setArtifactTrustError('')
    return getArtifactTrustPolicy()
      .then((res) => {
        setArtifactTrustPolicyState({
          verificationMode: String(res?.verificationMode || 'warn_unsigned'),
          allowedSigningKeyIds: Array.isArray(res?.allowedSigningKeyIds) ? res.allowedSigningKeyIds : [],
          allowedSignatureTypes: Array.isArray(res?.allowedSignatureTypes) ? res.allowedSignatureTypes : [],
          updatedAt: res?.updatedAt || '',
          updatedByUserId: res?.updatedByUserId || '',
        })
        return res
      })
      .catch((err) => {
        if (!silent) setArtifactTrustError(err.message || String(err))
        throw err
      })
  }

  function loadTrustedSigningKeys({ includeRetired = true, silent = false } = {}) {
    if (!canViewArtifactTrust) {
      setTrustedSigningKeys([])
      setTrustedSigningKeysLoaded(false)
      return Promise.resolve(null)
    }
    setTrustedSigningKeysLoading(true)
    if (!silent) setArtifactTrustError('')
    return listTrustedSigningKeys(includeRetired)
      .then((res) => {
        setTrustedSigningKeys(Array.isArray(res?.items) ? res.items : [])
        setTrustedSigningKeysLoaded(true)
        return res
      })
      .catch((err) => {
        if (!silent) setArtifactTrustError(err.message || String(err))
        throw err
      })
      .finally(() => setTrustedSigningKeysLoading(false))
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

  async function buildArtifactUploadFormData(form) {
    const formData = new FormData(form)
    const signatureFile = formData.get('signatureFile')
    formData.delete('signatureFile')
    const signatureType = String(formData.get('signatureType') || '').trim()

    if (signatureFile instanceof File && signatureFile.size > 0) {
      const signatureText = (await signatureFile.text()).trim()
      if (!signatureText) {
        throw new Error('signature file is empty')
      }
      formData.set('signature', signatureText)
      formData.set('signatureType', signatureType || 'ed25519')
    } else {
      formData.delete('signature')
      formData.delete('signatureType')
    }

    const signatureKeyId = String(formData.get('signatureKeyId') || '').trim()
    if (signatureKeyId) {
      formData.set('signatureKeyId', signatureKeyId)
    } else {
      formData.delete('signatureKeyId')
    }

    if (artifactTrustPolicy.verificationMode === 'require_verified') {
      if (!(signatureFile instanceof File && signatureFile.size > 0)) {
        throw new Error('trusted upload required: detached signature file is required')
      }
      if (!signatureKeyId) {
        throw new Error('trusted upload required: signing key ID is required')
      }
    }
    if (signatureKeyId && artifactTrustPolicy.allowedSigningKeyIds.length > 0 && !artifactTrustPolicy.allowedSigningKeyIds.includes(signatureKeyId)) {
      throw new Error('signing key is not allowed by the current trust policy')
    }
    if (signatureFile instanceof File && signatureFile.size > 0) {
      const effectiveSignatureType = String(formData.get('signatureType') || '').trim()
      if (artifactTrustPolicy.allowedSignatureTypes.length > 0 && !artifactTrustPolicy.allowedSignatureTypes.includes(effectiveSignatureType)) {
        throw new Error('signature type is not allowed by the current trust policy')
      }
    }

    return formData
  }

  async function handleUpload(e) {
    e.preventDefault()
    if (!canManageArtifacts) return
    const form = e.currentTarget
    setUploadStatus('Uploading...')
    buildArtifactUploadFormData(form)
      .then((formData) => uploadArtifact(formData))
      .then((resp) => {
        if (resp.duplicate) {
          setUploadStatus(`Already exists — existing artifact ${resp.artifactId?.slice(0, 8) ?? ''} returned (idempotent, nothing new created).`)
        } else {
          setUploadStatus(`Uploaded artifact ${resp.artifactId}`)
        }
        form.reset()
        loadArtifacts()
        setArtifactUploadOpen(false)
      })
      .catch((err) => setUploadStatus(err.message || String(err)))
  }

  function handleOpenArtifactUpload() {
    if (!canManageArtifacts) return
    loadArtifactTrustPolicy({ silent: true }).catch(() => {})
    loadTrustedSigningKeys({ silent: true }).catch(() => {})
    setUploadStatus('')
    setArtifactUploadOpen(true)
  }

  async function handleSaveArtifactTrustPolicy() {
    if (!canManageArtifactTrust) return
    setArtifactTrustStatus('Saving trust policy...')
    setArtifactTrustError('')
    try {
      const res = await saveArtifactTrustPolicy({
        verificationMode: artifactTrustPolicy.verificationMode,
        allowedSigningKeyIds: artifactTrustPolicy.allowedSigningKeyIds,
        allowedSignatureTypes: artifactTrustPolicy.allowedSignatureTypes,
      })
      setArtifactTrustPolicyState({
        verificationMode: String(res?.verificationMode || 'warn_unsigned'),
        allowedSigningKeyIds: Array.isArray(res?.allowedSigningKeyIds) ? res.allowedSigningKeyIds : [],
        allowedSignatureTypes: Array.isArray(res?.allowedSignatureTypes) ? res.allowedSignatureTypes : [],
        updatedAt: res?.updatedAt || '',
        updatedByUserId: res?.updatedByUserId || '',
      })
      setArtifactTrustStatus('Trust policy updated')
    } catch (err) {
      setArtifactTrustError(err.message || String(err))
      setArtifactTrustStatus('')
    }
  }

  function handleOpenCreateTrustedSigningKey() {
    if (!canManageArtifactTrust) return
    setEditingTrustedSigningKeyId('')
    setTrustedSigningKeyForm({
      displayName: '',
      algorithm: 'ed25519',
      publicKeyPem: '',
      notes: '',
    })
    setArtifactTrustStatus('')
    setTrustedSigningKeyModalOpen(true)
  }

  function handleStartEditTrustedSigningKey(key) {
    if (!canManageArtifactTrust || !key) return
    setEditingTrustedSigningKeyId(key.keyId)
    setTrustedSigningKeyForm({
      displayName: key.displayName || '',
      algorithm: key.algorithm || 'ed25519',
      publicKeyPem: key.publicKeyPem || '',
      notes: key.notes || '',
    })
    setArtifactTrustStatus('')
    setTrustedSigningKeyModalOpen(true)
  }

  async function handleSaveTrustedSigningKey(e) {
    e.preventDefault()
    if (!canManageArtifactTrust) return
    setArtifactTrustStatus(editingTrustedSigningKeyId ? 'Updating trusted key...' : 'Creating trusted key...')
    setArtifactTrustError('')
    try {
      if (editingTrustedSigningKeyId) {
        await updateTrustedSigningKey(editingTrustedSigningKeyId, {
          displayName: trustedSigningKeyForm.displayName,
          notes: trustedSigningKeyForm.notes,
        })
      } else {
        await createTrustedSigningKey({
          displayName: trustedSigningKeyForm.displayName,
          algorithm: trustedSigningKeyForm.algorithm,
          publicKeyPem: trustedSigningKeyForm.publicKeyPem,
          notes: trustedSigningKeyForm.notes,
        })
      }
      await loadTrustedSigningKeys({ silent: true })
      setArtifactTrustStatus(editingTrustedSigningKeyId ? 'Trusted key updated' : 'Trusted key created')
      setTrustedSigningKeyModalOpen(false)
      setEditingTrustedSigningKeyId('')
    } catch (err) {
      setArtifactTrustError(err.message || String(err))
      setArtifactTrustStatus('')
    }
  }

  async function handleRetireTrustedSigningKey(key) {
    if (!canManageArtifactTrust || !key) return
    const confirmed = window.confirm(`Retire trusted key ${key.displayName || key.keyId}? New uploads using this key will be rejected.`)
    if (!confirmed) return
    setArtifactTrustStatus('Retiring trusted key...')
    setArtifactTrustError('')
    try {
      await retireTrustedSigningKey(key.keyId)
      await loadTrustedSigningKeys({ silent: true })
      setArtifactTrustStatus('Trusted key retired')
    } catch (err) {
      setArtifactTrustError(err.message || String(err))
      setArtifactTrustStatus('')
    }
  }

  function handleDesiredDevice(e) {
    e.preventDefault()
    if (!canManageDesiredState) return
    setDesiredStatus('Setting desired state...')
    const { components, error } = buildComponentsPayloadForScope(deviceForm.components, 'device')
    if (error) {
      setDesiredStatus(error)
      return
    }
    const trackingConfigured = shouldQueueTrackingRefresh(deviceForm.components, 'device')
    if (Object.keys(components || {}).length === 0 && !deviceForm.checkinIntervalSec) {
      clearDesiredStateDevice(deviceForm.deviceId)
        .then(() => {
          setDesiredStatus('Device override cleared; group desired state will apply')
          loadDesired()
          setDeviceFormDirty(false)
        })
        .catch((err) => setDesiredStatus(err.message || String(err)))
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
        if (trackingConfigured) queueTrackingRefresh()
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

  async function handleDeprecateDuplicateLosers(artifactIds) {
    if (!canManageArtifacts || artifactIds.length === 0) return
    const ok = window.confirm(`Deprecate ${artifactIds.length} duplicate artifact(s) for this version?`)
    if (!ok) return
    setArtifactsStatus('Deprecating duplicate artifacts...')
    const retention = Number(artifactLifecyclePolicyInput) || undefined
    let deprecated = 0; let failed = 0
    for (const id of artifactIds) {
      try { await deprecateArtifact(id, retention); deprecated++ }
      catch { failed++ }
    }
    setArtifactsStatus(`Deprecated ${deprecated} duplicate(s)${failed ? `, ${failed} failed` : ''}.`)
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

  async function loadArtifactVulnScan(artifactId) {
    setArtifactVulnScans((prev) => ({ ...prev, [artifactId]: 'loading' }))
    try {
      const scan = await getLatestArtifactVulnScan(artifactId)
      setArtifactVulnScans((prev) => ({ ...prev, [artifactId]: scan }))
    } catch (err) {
      if (err.status === 404) {
        setArtifactVulnScans((prev) => ({ ...prev, [artifactId]: null }))
      } else {
        setArtifactVulnScans((prev) => ({ ...prev, [artifactId]: null }))
      }
    }
  }

  async function handleTriggerArtifactScan(artifactId) {
    try {
      await triggerArtifactScan(artifactId)
      setArtifactVulnScans((prev) => ({ ...prev, [artifactId]: 'loading' }))
      setTimeout(() => loadArtifactVulnScan(artifactId), 2000)
    } catch (err) {
      setArtifactsError(err.message || 'Failed to trigger scan')
    }
  }

  async function handleTriggerNessusSync() {
    setNessusSyncing(true)
    try {
      await triggerNessusSync()
      setTimeout(async () => {
        try {
          const status = await getNessusSyncStatus()
          setNessusSyncStatus(status)
        } catch (_) {}
        setNessusSyncing(false)
      }, 3000)
    } catch (err) {
      setNessusSyncing(false)
      setArtifactsError(err.message || 'Nessus sync failed')
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
    const { components, error } = buildComponentsPayloadForScope(groupMultiDesiredForm.components, 'group')
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
    if (shouldQueueTrackingRefresh(groupMultiDesiredForm.components, 'group')) queueTrackingRefresh()
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
    const { components, error } = buildComponentsPayloadForScope(groupDesiredForm.components, 'group')
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
      if (shouldQueueTrackingRefresh(groupDesiredForm.components, 'group')) queueTrackingRefresh()
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
  const artifactGroups = useMemo(() => {
    const map = new Map()
    artifacts.forEach((artifact) => {
      const name = String(artifact.name || 'unnamed')
      const type = normalizeArtifactType(artifact.type || 'app_bundle')
      const key = `${name}\u0000${type}`
      if (!map.has(key)) {
        map.set(key, { name, type, versions: [] })
      }
      map.get(key).versions.push(artifact)
    })
    return Array.from(map.entries())
      .map(([, group]) => {
        const versions = [...group.versions].sort((a, b) => a.version.localeCompare(b.version, undefined, { numeric: true }))
        const versionBuckets = new Map()
        versions.forEach(a => {
          if (!versionBuckets.has(a.version)) versionBuckets.set(a.version, [])
          versionBuckets.get(a.version).push(a)
        })
        const winnerIds = new Set()
        const losersByVersion = new Map()
        versionBuckets.forEach((members, version) => {
          if (members.length <= 1) return
          const sorted = [...members].sort((a, b) => {
            const tDiff = new Date(b.createdAt) - new Date(a.createdAt)
            if (tDiff !== 0) return tDiff
            return b.artifactId.localeCompare(a.artifactId)
          })
          winnerIds.add(sorted[0].artifactId)
          losersByVersion.set(version, sorted.slice(1).map(a => a.artifactId))
        })
        return { name: group.name, type: group.type, versions, winnerIds, losersByVersion }
      })
      .sort((a, b) => `${a.name}|${a.type}`.localeCompare(`${b.name}|${b.type}`))
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
    const effectivePolicy = resolveEffectiveTrustPolicy(row?.policy, artifactTrustPolicy)
    const byType = type ? filterArtifactGroupsByType(activeArtifactGroups, type) : activeArtifactGroups
    return filterArtifactGroupsByTrustPolicy(byType, effectivePolicy)
  }, [activeArtifactGroups, artifactPickerTarget, artifactTrustPolicy, deviceForm.components, groupDesiredForm.components, groupMultiDesiredForm.components])
  const artifactPickerPolicy = useMemo(() => {
    if (!artifactPickerTarget) return artifactTrustPolicy
    const { scope, index } = artifactPickerTarget
    const row = scope === 'group'
      ? groupDesiredForm.components?.[index]
      : scope === 'groupBulk'
        ? groupMultiDesiredForm.components?.[index]
        : deviceForm.components?.[index]
    return resolveEffectiveTrustPolicy(row?.policy, artifactTrustPolicy)
  }, [artifactPickerTarget, artifactTrustPolicy, deviceForm.components, groupDesiredForm.components, groupMultiDesiredForm.components])
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
  const activeTrustedSigningKeys = useMemo(
    () => trustedSigningKeys.filter((key) => String(key.state || '').toLowerCase() !== 'retired'),
    [trustedSigningKeys],
  )
  const trustOverrideEditorRow = useMemo(() => getTrustEditorRow(), [trustOverrideEditor, deviceForm.components, groupDesiredForm.components, groupMultiDesiredForm.components])
  const trustOverrideEffectivePolicy = useMemo(() => {
    const previewPolicy = mergeTrustOverrideIntoPolicy(trustOverrideEditorRow?.policy, trustOverrideForm)
    return resolveEffectiveTrustPolicy(previewPolicy, artifactTrustPolicy)
  }, [trustOverrideEditorRow, trustOverrideForm, artifactTrustPolicy])
  const trustOverrideSelectedArtifact = useMemo(
    () => artifacts.find((artifact) => artifact.artifactId === trustOverrideEditorRow?.artifactId) || null,
    [artifacts, trustOverrideEditorRow],
  )

  function artifactFamilyKey(name, type) {
    return `${String(name || '').trim().toLowerCase()}|${normalizeArtifactType(type)}`
  }

  function findArtifactFamily(name, type, groupsList = activeArtifactGroups) {
    const key = artifactFamilyKey(name, type)
    return (groupsList || []).find((group) => artifactFamilyKey(group.name, group.type) === key) || null
  }

  function getTrackingPreview(row, scope = 'group') {
    if (!row) return { state: 'none', message: 'No component selected.' }
    const mode = String(row.autoTrackMode || 'inherit').trim().toLowerCase()
    if (scope === 'device' && mode === 'inherit') {
      return {
        state: 'inherit-group',
        message: 'Inherits group desired state. Saving this row removes the device override for this component.',
      }
    }

    const trackingEnabled = scope === 'device' ? mode === 'enabled' : mode !== 'disabled'
    if (!trackingEnabled) {
      return { state: 'disabled', message: 'Tracking disabled. This component stays pinned to the selected artifact.' }
    }
    const waitingForGlobalEnable = scope !== 'device' && mode === 'inherit' && !releaseAutoUpdate.enabled

    const targetName = String(row.trackingName || '').trim()
    const targetType = normalizeArtifactType(row.artifactType)
    if (!targetName || !targetType) {
      return { state: 'misconfigured', message: 'Misconfigured target. Tracking requires an explicit artifact name and type.' }
    }

    const effectivePolicy = resolveEffectiveTrustPolicy(row.policy, artifactTrustPolicy)
    const family = findArtifactFamily(targetName, targetType, activeArtifactGroups)
    const currentArtifact = row.artifactId ? artifactByID[row.artifactId] : null
    const currentVersion = parseSemver4(currentArtifact?.version || row.desiredVersion)

    if (!family) {
      return {
        state: waitingForGlobalEnable ? 'global-disabled' : 'unavailable',
        familyKey: artifactFamilyKey(targetName, targetType),
        targetName,
        targetType,
        message: waitingForGlobalEnable
          ? 'Global auto-follow is disabled. This target is configured but will not advance until the global setting is enabled.'
          : 'No active artifacts match this tracking target.',
      }
    }

    const semverCandidates = family.versions.filter((artifact) => parseSemver4(artifact.version))
    if (semverCandidates.length === 0) {
      return {
        state: 'unavailable',
        familyKey: artifactFamilyKey(targetName, targetType),
        targetName,
        targetType,
        message: 'No eligible semver artifacts are available for this target.',
      }
    }

    const trustEligible = semverCandidates.filter((artifact) => artifactAllowedByTrustPolicy(artifact, effectivePolicy))
    if (trustEligible.length === 0) {
      return {
        state: waitingForGlobalEnable ? 'global-disabled' : 'trust-blocked',
        familyKey: artifactFamilyKey(targetName, targetType),
        targetName,
        targetType,
        message: waitingForGlobalEnable
          ? 'Global auto-follow is disabled. Matching artifacts exist, but no update will be applied until the global setting is enabled.'
          : 'Blocked by trust policy. Matching artifacts exist, but none satisfy the effective trust requirements.',
      }
    }

    const latestEligible = [...trustEligible].sort((left, right) => compareSemver4(left.version, right.version))[trustEligible.length - 1]
    const latestVersion = parseSemver4(latestEligible.version)
    const familyKey = artifactFamilyKey(targetName, targetType)

    if (!currentVersion) {
      return {
        state: waitingForGlobalEnable ? 'global-disabled' : 'advance',
        familyKey,
        targetName,
        targetType,
        latestEligible,
        message: waitingForGlobalEnable
          ? `Global auto-follow is disabled. Latest eligible artifact is ${latestEligible.version}.`
          : `Will resolve to ${latestEligible.version} when saved.`,
      }
    }

    const cmp = compareSemver4(latestVersion, currentVersion)
    if (cmp > 0) {
      return {
        state: waitingForGlobalEnable ? 'global-disabled' : 'advance',
        familyKey,
        targetName,
        targetType,
        latestEligible,
        message: waitingForGlobalEnable
          ? `Global auto-follow is disabled. A newer eligible artifact (${latestEligible.version}) is available.`
          : `Will auto-update to ${latestEligible.version} when saved.`,
      }
    }
    return {
      state: waitingForGlobalEnable ? 'global-disabled' : 'current',
      familyKey,
      targetName,
      targetType,
      latestEligible,
      message: waitingForGlobalEnable
        ? `Global auto-follow is disabled. Latest eligible artifact is ${latestEligible.version}.`
        : `Following latest eligible artifact ${latestEligible.version}.`,
    }
  }

  const trackedArtifactFamilies = useMemo(() => {
    const groupNameByID = groups.reduce((acc, group) => {
      acc[group.groupId] = group.name || group.groupId
      return acc
    }, {})
    const devicesByID = devices.reduce((acc, device) => {
      acc[device.deviceId] = device
      return acc
    }, {})
    const families = new Map()

    const addMember = (ownerType, ownerId, ownerLabel, componentKey, component, scope) => {
      const preview = getTrackingPreview(component, scope)
      if (!preview.familyKey) return
      if (!families.has(preview.familyKey)) {
        families.set(preview.familyKey, {
          familyKey: preview.familyKey,
          targetName: preview.targetName,
          targetType: preview.targetType,
          members: [],
        })
      }
      families.get(preview.familyKey).members.push({
        ownerType,
        ownerId,
        ownerLabel,
        componentKey,
        message: preview.message,
        state: preview.state,
        latestEligible: preview.latestEligible || null,
      })
    }

    for (const groupDesired of desiredState.groups || []) {
      for (const [componentKey, component] of Object.entries(groupDesired.components || {})) {
        addMember('group', groupDesired.groupId, groupNameByID[groupDesired.groupId] || groupDesired.groupId, componentKey, component, 'group')
      }
    }
    for (const deviceDesired of desiredState.devices || []) {
      for (const [componentKey, component] of Object.entries(deviceDesired.components || {})) {
        addMember('device', deviceDesired.deviceId, devicesByID[deviceDesired.deviceId]?.deviceId || deviceDesired.deviceId, componentKey, component, 'device')
      }
    }
    return families
  }, [desiredState, devices, groups, activeArtifactGroups, artifactByID, artifactTrustPolicy, releaseAutoUpdate.enabled])

  const artifactTrackingDetail = useMemo(
    () => trackedArtifactFamilies.get(artifactTrackingDetailKey) || null,
    [artifactTrackingDetailKey, trackedArtifactFamilies],
  )

  useEffect(() => {
    if (artifactTrackingDetailKey && !trackedArtifactFamilies.has(artifactTrackingDetailKey)) {
      setArtifactTrackingDetailKey('')
    }
  }, [artifactTrackingDetailKey, trackedArtifactFamilies])

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
          <div className="brand">{brandName}</div>
          <div className="status">Checking authentication...</div>
        </div>
      </div>
    )
  }

  if (authStatus.loaded && authStatus.enabled && !authToken) {
    return (
      <div className="login-screen">
        <div className="login-card">
          <div className="brand">{brandName}</div>
          <div className="auth-toggle">
            <button
              className={`tab ${authView === 'login' ? 'active' : ''}`}
              onClick={() => setAuthView('login')}
            >
              Sign in
            </button>
            {authStatus.mode === 'local' && (
              <button
                className={`tab ${authView === 'recover' ? 'active' : ''}`}
                onClick={() => setAuthView('recover')}
              >
                Recovery code
              </button>
            )}
            {authStatus.mode === 'local' && (
              <button
                className={`tab ${authView === 'reset-token' ? 'active' : ''}`}
                onClick={() => setAuthView('reset-token')}
              >
                Reset token
              </button>
            )}
            {authStatus.mode === 'local' && authStatus.smtpEnabled && (
              <button
                className={`tab ${authView === 'forgot-password' ? 'active' : ''}`}
                onClick={() => setAuthView('forgot-password')}
              >
                Forgot password
              </button>
            )}
            <button
              className={`tab ${authView === 'register' ? 'active' : ''}`}
              onClick={() => setAuthView('register')}
            >
              Use voucher
            </button>
            {authStatus.ldapEnabled && (
              <button
                className={`tab ${authView === 'ldap' ? 'active' : ''}`}
                onClick={() => setAuthView('ldap')}
              >
                LDAP / AD
              </button>
            )}
          </div>
          {authView === 'ldap' ? (
            <div className="form">
              <label>Username</label>
              <input
                value={ldapForm.username}
                onChange={(e) => setLdapForm((prev) => ({ ...prev, username: e.target.value }))}
                placeholder="alice"
                autoComplete="username"
              />
              <label>Password</label>
              <input
                type="password"
                value={ldapForm.password}
                onChange={(e) => setLdapForm((prev) => ({ ...prev, password: e.target.value }))}
                autoComplete="current-password"
              />
              <button className="button" onClick={doLdapLogin}>
                Sign in with LDAP
              </button>
              {ldapStatus && <div className="status">{ldapStatus}</div>}
            </div>
          ) : authView === 'login' ? (
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
          ) : authView === 'recover' ? (
            <div className="form">
              <label>Email</label>
              <input
                value={recoveryForm.email}
                onChange={(e) => setRecoveryForm((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="user@example.com"
              />
              <label>Recovery code</label>
              <input
                value={recoveryForm.recoveryCode}
                onChange={(e) => setRecoveryForm((prev) => ({ ...prev, recoveryCode: e.target.value }))}
                placeholder="ABCD-EFGH-IJKL-MNOP"
              />
              <label>New password</label>
              <input
                type="password"
                value={recoveryForm.newPassword}
                onChange={(e) => setRecoveryForm((prev) => ({ ...prev, newPassword: e.target.value }))}
              />
              <button className="button" onClick={doResetWithRecoveryCode}>
                Reset password
              </button>
            </div>
          ) : authView === 'reset-token' ? (
            <div className="form">
              <label>Email</label>
              <input
                value={resetTokenForm.email}
                onChange={(e) => setResetTokenForm((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="user@example.com"
              />
              <label>Reset token</label>
              <input
                value={resetTokenForm.resetToken}
                onChange={(e) => setResetTokenForm((prev) => ({ ...prev, resetToken: e.target.value }))}
                placeholder="paste reset token"
              />
              <label>New password</label>
              <input
                type="password"
                value={resetTokenForm.newPassword}
                onChange={(e) => setResetTokenForm((prev) => ({ ...prev, newPassword: e.target.value }))}
              />
              <button className="button" onClick={doResetWithPasswordResetToken}>
                Reset password
              </button>
            </div>
          ) : authView === 'forgot-password' ? (
            <div className="form">
              <label>Email</label>
              <input
                value={forgotPasswordForm.email}
                onChange={(e) => setForgotPasswordForm((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="user@example.com"
                autoComplete="email"
              />
              <button className="button" onClick={doForgotPassword}>
                Send reset link
              </button>
              {forgotPasswordStatus && <div className="status">{forgotPasswordStatus}</div>}
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
          {recoveryStatus && <div className="status">{recoveryStatus}</div>}
          {resetTokenStatus && <div className="status">{resetTokenStatus}</div>}
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
        <div className="brand">Parcel</div>
        <nav className="nav">
          {visibleNav.map((item) => {
            const pendingCount = item.id === 'security'
              ? pendingEnrollments.filter((e) => e.status === 'pending').length
              : 0
            return (
              <a
                key={item.id}
                href={`#${item.id}`}
                className={view === item.id ? 'active' : ''}
                onClick={() => setView(item.id)}
              >
                {navIcons[item.icon]}
                {item.label}
                {pendingCount > 0 && <span className="nav-badge">{pendingCount}</span>}
              </a>
            )
          })}
        </nav>
        <div className="sidebar-footer">
          {!featureFlags.embeddedMode && (
            <div className="env">
              API: {apiProxy ? `proxy → ${apiBaseUrl}` : apiBaseUrl}
              {simulateProd ? ' · prod-sim' : ''}
            </div>
          )}
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

            <DevicesSection
              {...{
                allDevicesSelected,
                canDecommissionDevices,
                clearDeviceSelection,
                deviceSourceFor,
                deviceStatusFilter,
                devicesError,
                devicesLoading,
                filteredDevices,
                handleBulkDecommissionSelectedDevices,
                handleDeviceRowSelect,
                handleSelectAllDevices,
                loadDevices,
                selectedDeviceId,
                selectedDeviceIds,
                selectedDeviceSet,
                setDeviceDrawerOpen,
                setDeviceFormDirty,
                setDeviceStatusFilter,
                setSelectedDeviceId,
                someDevicesSelected,
              }}
            />

            <GroupsSection
              {...{
                allGroupsSelected,
                canDecommissionDevices,
                canEditDeviceLabels,
                canManageDesiredState,
                canManageGroups,
                clearGroupSelection,
                formatSelector,
                groupBatchError,
                groupBatchStatus,
                groupCounts,
                groupDeviceList,
                groups,
                groupsError,
                handleBulkDeleteSelectedGroups,
                handleDeleteGroup,
                handleGroupDeviceToggle,
                handleGroupRowSelect,
                handleSelectAllGroups,
                loadGroups,
                normalizeObject,
                openGroupMultiDesired,
                openGroupMultiEdit,
                selectedGroup,
                selectedGroupId,
                selectedGroupIds,
                selectedGroupSelector,
                selectedGroupSet,
                selectorToForm,
                setGroupBulkError,
                setGroupBulkOpen,
                setGroupBulkStatus,
                setGroupDesiredOpen,
                setGroupForm,
                setGroupModalOpen,
                setSelectedGroupId,
                someGroupsSelected,
              }}
            />

            <section id="artifacts" className="card">
              <div className="section-header">
                <h2>Artifacts</h2>
                <div className="inline-row">
                  <button onClick={handleOpenArtifactUpload} className="button" disabled={!canManageArtifacts}>
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
                      className="button danger"
                      onClick={handleBulkDeprecateSelectedArtifacts}
                      disabled={!canManageArtifacts || selectedArtifactIds.length === 0}
                    >
                      Deprecate Selected
                    </button>
                    <button
                      className="button danger"
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
                      <th>Trust</th>
                      <th>Tracking</th>
                      <th>Refs</th>
                      <th>Vulns</th>
                      <th>Delete After</th>
                      <th>Created</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {artifactGroups.map((group) => (
                      group.versions.map((a, idx) => {
                        const lifecycle = normalizeArtifactStatus(a.status)
                        const trackingKey = artifactFamilyKey(a.name, a.type)
                        const trackedFamily = trackedArtifactFamilies.get(trackingKey)
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
                              {group.winnerIds.has(a.artifactId) && (
                                <span className="pill canonical">canonical</span>
                              )}
                              {group.losersByVersion.has(a.version) && !group.winnerIds.has(a.artifactId) && (
                                <span className="pill duplicate">duplicate</span>
                              )}
                            </td>
                            <td>
                              <div className="artifact-trust-cell">
                                <span className={`pill ${normalizeVerificationStatus(a.verificationStatus, a)}`}>
                                  {verificationPillLabel(a)}
                                </span>
                                <div className="detail-note">{artifactSignerSummary(a)}</div>
                              </div>
                            </td>
                            <td>
                              {trackedFamily ? (
                                <div className="artifact-trust-cell">
                                  <span className="pill info">tracked</span>
                                  <button
                                    className="button ghost"
                                    type="button"
                                    onClick={() => setArtifactTrackingDetailKey(trackingKey)}
                                  >
                                    {trackedFamily.members.length} target{trackedFamily.members.length === 1 ? '' : 's'}
                                  </button>
                                </div>
                              ) : '—'}
                            </td>
                            <td>{refs}</td>
                            <td>
                              {(() => {
                                const scan = artifactVulnScans[a.artifactId]
                                if (!scan || scan === 'loading') {
                                  return (
                                    <button
                                      className="button ghost muted"
                                      style={{ fontSize: '0.75rem', padding: '2px 6px' }}
                                      onClick={() => { loadArtifactVulnScan(a.artifactId); setArtifactVulnPanelId(a.artifactId) }}
                                      title="Load vulnerability scan"
                                    >
                                      {scan === 'loading' ? '…' : 'scan?'}
                                    </button>
                                  )
                                }
                                const c = scan.severityCounts || {}
                                const crit = c.critical || 0
                                const high = c.high || 0
                                return (
                                  <button
                                    className="button ghost"
                                    style={{ fontSize: '0.75rem', padding: '2px 6px' }}
                                    onClick={() => setArtifactVulnPanelId(artifactVulnPanelId === a.artifactId ? '' : a.artifactId)}
                                    title="View vulnerability scan details"
                                  >
                                    {crit > 0 && <span className="pill pill-critical" style={{ marginRight: 2 }}>{crit}C</span>}
                                    {high > 0 && <span className="pill pill-high" style={{ marginRight: 2 }}>{high}H</span>}
                                    {crit === 0 && high === 0 && <span className="pill info">{scan.scanStatus || 'ok'}</span>}
                                  </button>
                                )
                              })()}
                            </td>
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
                              {group.winnerIds.has(a.artifactId) && group.losersByVersion.has(a.version) && (
                                <button
                                  className="button ghost"
                                  onClick={() => handleDeprecateDuplicateLosers(group.losersByVersion.get(a.version))}
                                  disabled={!canManageArtifacts}
                                  title="Deprecate non-canonical duplicates for this version"
                                >
                                  Deprecate duplicates
                                </button>
                              )}
                            </td>
                          </tr>
                        )
                      })
                    ))}
                    {artifactGroups.length === 0 && (
                      <tr>
                        <td colSpan={13}>No artifacts uploaded yet.</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
              {artifactVulnPanelId && (() => {
                const scan = artifactVulnScans[artifactVulnPanelId]
                const artifact = artifacts.find((a) => a.artifactId === artifactVulnPanelId)
                return (
                  <div className="artifact-tracking-panel">
                    <div className="section-header">
                      <h3>Vulnerability Scan: {artifact?.name} {artifact?.version}</h3>
                      <div style={{ display: 'flex', gap: 8 }}>
                        {canManageArtifacts && (
                          <button className="button ghost" onClick={() => handleTriggerArtifactScan(artifactVulnPanelId)}>
                            Rescan
                          </button>
                        )}
                        <button className="button ghost" onClick={() => setArtifactVulnPanelId('')}>Close</button>
                      </div>
                    </div>
                    {!scan || scan === 'loading' ? (
                      <p>Loading…</p>
                    ) : (
                      <div>
                        <div style={{ marginBottom: 8 }}>
                          <strong>Status:</strong> {scan.scanStatus}{scan.scannerType ? ` (${scan.scannerType}${scan.scannerVersion ? ' ' + scan.scannerVersion : ''})` : ''}
                          {scan.scannedAt && <span style={{ marginLeft: 8, color: 'var(--text-muted)' }}>Scanned {formatTime(scan.scannedAt)}</span>}
                          {scan.errorMessage && <span style={{ marginLeft: 8, color: 'var(--color-danger)' }}>{scan.errorMessage}</span>}
                        </div>
                        {scan.severityCounts && (
                          <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                            {['critical','high','medium','low','unknown'].map((sev) => {
                              const n = scan.severityCounts[sev] || 0
                              if (n === 0) return null
                              return <span key={sev} className={`pill pill-${sev}`}>{n} {sev}</span>
                            })}
                          </div>
                        )}
                        {scan.findings && scan.findings.length > 0 ? (
                          <div className="table-wrap">
                            <table>
                              <thead>
                                <tr><th>CVE</th><th>Severity</th><th>Package</th><th>Version</th><th>Fixed In</th></tr>
                              </thead>
                              <tbody>
                                {scan.findings.map((f, i) => (
                                  <tr key={i}>
                                    <td className="mono">{f.id}</td>
                                    <td><span className={`pill pill-${(f.severity||'unknown').toLowerCase()}`}>{f.severity}</span></td>
                                    <td>{f.package}</td>
                                    <td className="mono">{f.version}</td>
                                    <td className="mono">{f.fixedIn || '—'}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        ) : (
                          <p style={{ color: 'var(--text-muted)' }}>No findings.</p>
                        )}
                      </div>
                    )}
                  </div>
                )
              })()}

              {artifactTrackingDetail && (
                <div className="artifact-tracking-panel">
                  <div className="section-header">
                    <h3>
                      Tracking detail: {artifactTrackingDetail.targetName} ({artifactTrackingDetail.targetType})
                    </h3>
                    <button className="button ghost" type="button" onClick={() => setArtifactTrackingDetailKey('')}>
                      Close
                    </button>
                  </div>
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>Owner</th>
                          <th>Component</th>
                          <th>Status</th>
                          <th>Latest Eligible</th>
                        </tr>
                      </thead>
                      <tbody>
                        {artifactTrackingDetail.members.map((member) => (
                          <tr key={`${member.ownerType}-${member.ownerId}-${member.componentKey}`}>
                            <td>{member.ownerType === 'group' ? `Group: ${member.ownerLabel}` : `Device: ${member.ownerLabel}`}</td>
                            <td>{member.componentKey}</td>
                            <td>{member.message}</td>
                            <td>{member.latestEligible?.version || '—'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
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

        <SecurityPage
          {...{
            activeTrustedSigningKeys,
            artifactLifecyclePolicy,
            artifactLifecyclePolicyInput,
            artifactLifecycleStatus,
            artifactTrustError,
            artifactTrustPolicy,
            artifactTrustStatus,
            authError,
            authStatus,
            authToken,
            authUser,
            backupError,
            backupMessage,
            backupStatus,
            backups,
            canApplyUpgrade,
            canManageArtifactLifecycle,
            canManageArtifactTrust,
            canManageBackups,
            canManageMaintenance,
            canManagePendingEnrollments,
            canManageReleaseAutoUpdate,
            canManageUsers,
            canRotate,
            canViewArtifactTrust,
            canViewBackups,
            doLogin,
            doLogout,
            doDownloadRecoveryCodes,
            doGenerateRecoveryCodes,
            handleIssuePasswordResetToken,
            handleSendUserInvite,
            enrollmentProfiles,
            enrollmentProfilesError,
            enrollmentProfilesLoading,
            enrollmentProfilesStatus,
            formatDurationSeconds,
            formatObjectSummary,
            formatTime,
            handleApprovePendingEnrollment,
            handleCleanupRotation,
            handleCopyEnrollmentProfileToken,
            handleDenyPendingEnrollment,
            handleDownloadEnrollmentProfileToken,
            handleOpenCreateEnrollmentProfile,
            handleOpenCreateTrustedSigningKey,
            handleReloadRotation,
            handleResetPendingEnrollment,
            handleRestore,
            handleRetireTrustedSigningKey,
            handleRotateEnrollmentProfile,
            handleRotateRotation,
            handleRunReleaseAutoUpdate,
            handleSaveArtifactLifecyclePolicy,
            handleSaveArtifactTrustPolicy,
            handleSaveReleaseAutoUpdateSettings,
            handleSetEnrollmentProfileDisabled,
            handleStartBackup,
            handleStartEditEnrollmentProfile,
            handleStartEditTrustedSigningKey,
            latestEnrollmentProfileToken,
            loadArtifactLifecyclePolicy,
            loadArtifactLifecycleStatus,
            loadArtifactTrustPolicy,
            loadBackups,
            loadEnrollmentProfiles,
            loadMaintenance,
            loadPendingEnrollments,
            loadReleaseAutoUpdate,
            loadRotationStatus,
            loadTrustedSigningKeys,
            loadUpgrade,
            loadUpgradeAvailable,
            loadUpgradePreflight,
            loadUsers,
            loginForm,
            loginStatus,
            maintenance,
            pendingEnrollments,
            pendingEnrollmentsError,
            pendingEnrollmentsFilter,
            pendingEnrollmentsLoading,
            pendingEnrollmentsStatus,
            preflightOk,
            releaseAutoUpdate,
            releaseAutoUpdateSaving,
            releaseAutoUpdateStatus,
            restoreStatus,
            recoveryCodes,
            recoveryCodesError,
            recoveryCodesGeneratedAt,
            recoveryCodesStatus,
            passwordResetIssueForm,
            passwordResetTokenError,
            passwordResetTokenStatus,
            passwordResetTokenTarget,
            passwordResetTokenValue,
            rotationError,
            rotationLoading,
            rotationMessage,
            rotationStatus,
            selectedBackupId,
            setArtifactLifecyclePolicyInput,
            setArtifactTrustPolicyState,
            setLoginForm,
            setPasswordResetIssueForm,
            setPendingEnrollmentsFilter,
            setReleaseAutoUpdate,
            setSelectedBackupId,
            setUserForm,
            setVoucherForm,
            signatureTypeOptions,
            startUpgrade,
            submitUser,
            submitVoucher,
            toggleMaintenance,
            toggleRole,
            toggleVoucherRole,
            trustedSigningKeys,
            trustedSigningKeysLoading,
            upgrade,
            upgradeAvailable,
            upgradeAvailableError,
            upgradePreflight,
            upgradePreflightError,
            upgradePreflightStatus,
            upgradeReady,
            userForm,
            users,
            usersError,
            usersStatus,
            view,
            voucherForm,
            voucherStatus,
            voucherToken,
          }}
        />

        <SettingsPage
          {...{
            artifactLifecyclePolicy,
            artifactLifecyclePolicyInput,
            artifactLifecycleStatus,
            backupError,
            backupMessage,
            backupStatus,
            backups,
            canApplyUpgrade,
            canManageArtifactLifecycle,
            canManageBackups,
            canManageMaintenance,
            canManageReleaseAutoUpdate,
            canViewBackups,
            formatDurationSeconds,
            formatTime,
            handleRestore,
            handleRunReleaseAutoUpdate,
            handleSaveArtifactLifecyclePolicy,
            handleSaveReleaseAutoUpdateSettings,
            handleStartBackup,
            loadArtifactLifecyclePolicy,
            loadArtifactLifecycleStatus,
            loadBackups,
            loadMaintenance,
            loadReleaseAutoUpdate,
            loadUpgrade,
            loadUpgradeAvailable,
            loadUpgradePreflight,
            maintenance,
            preflightOk,
            releaseAutoUpdate,
            releaseAutoUpdateSaving,
            releaseAutoUpdateStatus,
            restoreStatus,
            selectedBackupId,
            setArtifactLifecyclePolicyInput,
            setReleaseAutoUpdate,
            setSelectedBackupId,
            startUpgrade,
            toggleMaintenance,
            upgrade,
            upgradeAvailable,
            upgradeAvailableError,
            upgradePreflight,
            upgradePreflightError,
            upgradePreflightStatus,
            upgradeReady,
            view,
            nessusSyncStatus,
            nessusSyncing,
            handleTriggerNessusSync,
            canAdmin: permissions.admin,
          }}
        />
      </main>

      <DeviceDrawer
        {...{
          activeArtifactGroups,
          addDeviceComponent,
          artifactAllowedByTrustPolicy,
          artifactGroups,
          artifactSignerSummary,
          artifactTrustPolicy,
          artifacts,
          buildComponentRows,
          canDecommissionDevices,
          canManageDesiredState,
          deviceTrackingModes,
          desiredError,
          deviceDetail,
          deviceDetailError,
          deviceDrawerOpen,
          deviceForm,
          downloadLogs,
          drawerResizingRef,
          drawerWidth,
          filterArtifactGroupsByTrustPolicy,
          filterArtifactGroupsByType,
          getTrackingPreview,
          handleClearDeviceOverride,
          handleDeleteDevice,
          handleDesiredDevice,
          lastAppliedArtifact,
          normalizeArtifactType,
          openArtifactPicker,
          openTrustOverrideEditor,
          preApplyBadgeClass,
          removeDeviceComponent,
          resolveEffectiveTrustPolicy,
          selectedDesired,
          selectedDeviceId,
          setDeviceDrawerOpen,
          setDeviceForm,
          setDeviceFormDirty,
          trustPolicySummary,
          updateDeviceComponent,
          verificationPillLabel,
          deviceVulnScan: selectedDeviceId ? (artifactVulnScans[selectedDeviceId] || null) : null,
          onLoadDeviceVulnScan: async (deviceId) => {
            setArtifactVulnScans((prev) => ({ ...prev, [deviceId]: 'loading' }))
            try {
              const scan = await getLatestDeviceVulnScan(deviceId)
              setArtifactVulnScans((prev) => ({ ...prev, [deviceId]: scan }))
            } catch (err) {
              if (err.status === 404) {
                setArtifactVulnScans((prev) => ({ ...prev, [deviceId]: null }))
              }
            }
          },
        }}
      />

      <GlobalPage
        view={view}
        globalPlaneUrl={globalPlaneUrl}
        onChangeGlobalPlaneUrl={(url) => {
          setGlobalPlaneUrl(url)
          try { localStorage.setItem('hwops_global_plane_url', url) } catch {}
        }}
        formatTime={formatTime}
      />

      {extraViews.map(({ id, component: ExtraPage }) =>
        view === id && (
          <ExtraPage
            key={id}
            view={view}
            permissions={permissions}
            authUser={authUser}
          />
        )
      )}

      <TrustedSigningKeyModal
        {...{
          canManageArtifactTrust,
          editingTrustedSigningKeyId,
          handleSaveTrustedSigningKey,
          setTrustedSigningKeyForm,
          setTrustedSigningKeyModalOpen,
          trustedSigningKeyForm,
          trustedSigningKeyModalOpen,
        }}
      />

      <EnrollmentProfileCreateModal
        {...{
          canManagePendingEnrollments,
          enrollmentProfileCreateOpen,
          enrollmentProfileForm,
          handleCreateEnrollmentProfile,
          setEnrollmentProfileCreateOpen,
          setEnrollmentProfileForm,
        }}
      />

      <EnrollmentProfileEditModal
        {...{
          canManagePendingEnrollments,
          editingEnrollmentProfileId,
          enrollmentProfileEditForm,
          handleCancelEditEnrollmentProfile,
          handleUpdateEnrollmentProfile,
          setEnrollmentProfileEditForm,
        }}
      />

      <ArtifactPickerModal
        {...{
          artifactAllowedByTrustPolicy,
          artifactModalGroups,
          artifactModalOpen,
          artifactPickerPolicy,
          artifactPickerTarget,
          artifactSignerSummary,
          canManageDesiredState,
          normalizeArtifactType,
          normalizeVerificationStatus,
          setArtifactModalOpen,
          setArtifactPickerTarget,
          updateDeviceComponent,
          updateGroupComponent,
          updateGroupMultiDesiredComponent,
          verificationPillLabel,
        }}
      />

      <ArtifactUploadModal
        {...{
          activeTrustedSigningKeys,
          artifactTrustError,
          artifactTrustPolicy,
          artifactUploadOpen,
          canManageArtifacts,
          handleUpload,
          setArtifactUploadOpen,
          signatureTypeOptions,
          uploadStatus,
        }}
      />

      <TrustOverrideModal
        {...{
          activeTrustedSigningKeys,
          artifactAllowedByTrustPolicy,
          artifactSignerSummary,
          artifactTrustPolicy,
          canManageDesiredState,
          closeTrustOverrideEditor,
          saveTrustOverrideEditor,
          setTrustOverrideError,
          setTrustOverrideForm,
          signatureTypeOptions,
          trustOverrideEditor,
          trustOverrideEditorRow,
          trustOverrideEffectivePolicy,
          trustOverrideError,
          trustOverrideForm,
          trustOverrideModes,
          trustOverrideSelectedArtifact,
          trustPolicyStrictness,
          verificationModeLabel,
          verificationPillLabel,
        }}
      />

      <GroupModal
        {...{
          canManageGroups,
          groupForm,
          groupModalOpen,
          groupsStatus,
          handleSaveGroup,
          setGroupForm,
          setGroupModalOpen,
        }}
      />

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

      <GroupMultiDesiredModal
        {...{
          activeArtifactGroups,
          addGroupMultiDesiredComponent,
          artifactAllowedByTrustPolicy,
          artifactGroups,
          artifactSignerSummary,
          artifactTrustPolicy,
          artifacts,
          canManageDesiredState,
          filterArtifactGroupsByTrustPolicy,
          filterArtifactGroupsByType,
          getTrackingPreview,
          groupTrackingModes,
          groupMultiDesiredError,
          groupMultiDesiredForm,
          groupMultiDesiredOpen,
          groupMultiDesiredStatus,
          handleApplyGroupMultiDesired,
          normalizeArtifactType,
          openArtifactPicker,
          openTrustOverrideEditor,
          removeGroupMultiDesiredComponent,
          resolveEffectiveTrustPolicy,
          selectedGroupIds,
          setGroupMultiDesiredForm,
          setGroupMultiDesiredOpen,
          trustPolicySummary,
          updateGroupMultiDesiredComponent,
          verificationPillLabel,
        }}
      />

      <BulkGroupManagementModal
        {...{
          canManageGroups,
          downloadGroupBulkRollback,
          downloadGroupBulkTemplate,
          formatSelector,
          groupBulkApplying,
          groupBulkCsv,
          groupBulkError,
          groupBulkOpen,
          groupBulkPreview,
          groupBulkRollbackCsv,
          groupBulkStatus,
          handleApplyGroupBulk,
          handleGroupBulkFileChange,
          handlePreviewGroupBulk,
          setGroupBulkCsv,
          setGroupBulkError,
          setGroupBulkOpen,
          setGroupBulkPreview,
          setGroupBulkRollbackCsv,
          setGroupBulkStatus,
        }}
      />

      <GroupDesiredModal
        {...{
          activeArtifactGroups,
          addGroupComponent,
          artifactAllowedByTrustPolicy,
          artifactGroups,
          artifactSignerSummary,
          artifactTrustPolicy,
          artifacts,
          canManageDesiredState,
          filterArtifactGroupsByTrustPolicy,
          filterArtifactGroupsByType,
          getTrackingPreview,
          groupTrackingModes,
          groupDesiredForm,
          groupDesiredOpen,
          groupsStatus,
          handleClearGroupDesired,
          handleGroupDesired,
          normalizeArtifactType,
          openArtifactPicker,
          openTrustOverrideEditor,
          removeGroupComponent,
          resolveEffectiveTrustPolicy,
          selectedGroupDesired,
          setGroupDesiredForm,
          setGroupDesiredOpen,
          trustPolicySummary,
          updateGroupComponent,
          verificationPillLabel,
        }}
      />
      <datalist id="artifact-types">
        {componentTypes.map((comp) => (
          <option key={comp.id} value={comp.id}>{comp.label}</option>
        ))}
      </datalist>
    </div>
  )
}
