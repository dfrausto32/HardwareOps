export default function SettingsPage({
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
}) {
  return (
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
  )
}
