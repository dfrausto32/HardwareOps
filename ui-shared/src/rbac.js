const roleRank = {
  viewer: 1,
  operator: 2,
  admin: 3,
}

const viewRequirements = {
  dashboard: 'viewer',
  metrics: 'admin',
  logs: 'viewer',
  security: 'viewer',
  settings: 'viewer',
  global: 'admin',
}

const logsTabRequirements = {
  events: 'viewer',
  device: 'viewer',
  audit: 'admin',
}

function normalizeRoles(roles = []) {
  return roles
    .map((role) => String(role || '').trim().toLowerCase())
    .filter(Boolean)
}

export function hasRole(roles = [], required) {
  const normalizedRequired = String(required || '').trim().toLowerCase()
  if (!normalizedRequired) return true
  const requiredRank = roleRank[normalizedRequired] || 0
  const bestRank = normalizeRoles(roles).reduce((best, role) => {
    const rank = roleRank[role] || 0
    return rank > best ? rank : best
  }, 0)
  return bestRank >= requiredRank
}

export function firstAllowedKey(allowedMap, fallback = '') {
  return Object.entries(allowedMap || {}).find(([, allowed]) => Boolean(allowed))?.[0] || fallback
}

export function buildPermissionState({ authEnabled = false, roles = [] } = {}) {
  const normalizedRoles = normalizeRoles(roles)
  const can = (required) => !authEnabled || hasRole(normalizedRoles, required)

  return {
    isViewer: can('viewer'),
    isOperator: can('operator'),
    isAdmin: can('admin'),
    views: Object.fromEntries(
      Object.entries(viewRequirements).map(([key, required]) => [key, can(required)]),
    ),
    logsTabs: Object.fromEntries(
      Object.entries(logsTabRequirements).map(([key, required]) => [key, can(required)]),
    ),
    canViewMetrics: can('admin'),
    canViewAudit: can('admin'),
    canManageUsers: can('admin'),
    canManageArtifacts: can('operator'),
    canManageArtifactLifecycle: can('admin'),
    canManagePendingEnrollments: can('operator'),
    canRotateCertificates: can('operator'),
    canManageDesiredState: can('operator'),
    canManageGroups: can('operator'),
    canEditDeviceLabels: can('operator'),
    canDecommissionDevices: can('admin'),
    canManageMaintenance: can('admin'),
    canViewBackups: can('admin'),
    canManageBackups: can('admin'),
    canManageReleaseAutoUpdate: can('admin'),
    canManageAuditRetention: can('admin'),
    canManageEventRetention: can('admin'),
    canApplyUpgrade: can('admin'),
  }
}
