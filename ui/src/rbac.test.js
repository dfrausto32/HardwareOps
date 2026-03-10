import test from 'node:test'
import assert from 'node:assert/strict'

import { buildPermissionState, hasRole } from './rbac.js'

test('role hierarchy matches backend rank ordering', () => {
  assert.equal(hasRole(['viewer'], 'viewer'), true)
  assert.equal(hasRole(['viewer'], 'operator'), false)
  assert.equal(hasRole(['operator'], 'viewer'), true)
  assert.equal(hasRole(['operator'], 'operator'), true)
  assert.equal(hasRole(['operator'], 'admin'), false)
  assert.equal(hasRole(['admin'], 'operator'), true)
})

test('viewer is read-only', () => {
  const permissions = buildPermissionState({ authEnabled: true, roles: ['viewer'] })

  assert.equal(permissions.views.dashboard, true)
  assert.equal(permissions.views.metrics, false)
  assert.equal(permissions.logsTabs.audit, false)
  assert.equal(permissions.canManageArtifacts, false)
  assert.equal(permissions.canManageDesiredState, false)
  assert.equal(permissions.canManageGroups, false)
  assert.equal(permissions.canEditDeviceLabels, false)
  assert.equal(permissions.canDecommissionDevices, false)
  assert.equal(permissions.canRotateCertificates, false)
  assert.equal(permissions.canManageUsers, false)
  assert.equal(permissions.canManageMaintenance, false)
  assert.equal(permissions.canViewBackups, false)
})

test('operator gets operator writes but not admin-only actions', () => {
  const permissions = buildPermissionState({ authEnabled: true, roles: ['operator'] })

  assert.equal(permissions.views.metrics, false)
  assert.equal(permissions.logsTabs.audit, false)
  assert.equal(permissions.canManageArtifacts, true)
  assert.equal(permissions.canManageDesiredState, true)
  assert.equal(permissions.canManageGroups, true)
  assert.equal(permissions.canEditDeviceLabels, true)
  assert.equal(permissions.canManagePendingEnrollments, true)
  assert.equal(permissions.canRotateCertificates, true)
  assert.equal(permissions.canManageArtifactLifecycle, false)
  assert.equal(permissions.canDecommissionDevices, false)
  assert.equal(permissions.canManageUsers, false)
  assert.equal(permissions.canManageMaintenance, false)
  assert.equal(permissions.canManageReleaseAutoUpdate, false)
})

test('admin gets admin-only reads and writes', () => {
  const permissions = buildPermissionState({ authEnabled: true, roles: ['admin'] })

  assert.equal(permissions.views.metrics, true)
  assert.equal(permissions.logsTabs.audit, true)
  assert.equal(permissions.canViewMetrics, true)
  assert.equal(permissions.canViewAudit, true)
  assert.equal(permissions.canManageUsers, true)
  assert.equal(permissions.canManageArtifactLifecycle, true)
  assert.equal(permissions.canDecommissionDevices, true)
  assert.equal(permissions.canManageMaintenance, true)
  assert.equal(permissions.canViewBackups, true)
  assert.equal(permissions.canManageReleaseAutoUpdate, true)
  assert.equal(permissions.canManageAuditRetention, true)
  assert.equal(permissions.canManageEventRetention, true)
  assert.equal(permissions.canApplyUpgrade, true)
})

test('auth-disabled mode stays fully open', () => {
  const permissions = buildPermissionState({ authEnabled: false, roles: [] })

  assert.equal(permissions.views.metrics, true)
  assert.equal(permissions.logsTabs.audit, true)
  assert.equal(permissions.canManageUsers, true)
  assert.equal(permissions.canManageArtifacts, true)
  assert.equal(permissions.canDecommissionDevices, true)
  assert.equal(permissions.canViewBackups, true)
})
