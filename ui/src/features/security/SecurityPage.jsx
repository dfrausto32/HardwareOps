export default function SecurityPage({
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
  doDownloadRecoveryCodes,
  doGenerateRecoveryCodes,
  doLogin,
  doLogout,
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
  recoveryCodes,
  recoveryCodesError,
  recoveryCodesGeneratedAt,
  recoveryCodesStatus,
  releaseAutoUpdate,
  releaseAutoUpdateSaving,
  releaseAutoUpdateStatus,
  restoreStatus,
  rotationError,
  rotationLoading,
  rotationMessage,
  rotationStatus,
  selectedBackupId,
  setArtifactLifecyclePolicyInput,
  setArtifactTrustPolicyState,
  setLoginForm,
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
}) {
  return (
    <>
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
                    {authStatus.mode === 'local' && (
                      <div>
                        <div className="detail-label">Recovery codes</div>
                        <div className="detail-value">
                          {authUser.recoveryCodesConfigured
                            ? `Configured${authUser.recoveryCodesGeneratedAt ? ` (${formatTime(authUser.recoveryCodesGeneratedAt)})` : ''}`
                            : 'Not configured'}
                        </div>
                      </div>
                    )}
                    <div className="full">
                      {authStatus.mode === 'local' && (
                        <button className="button ghost" onClick={doGenerateRecoveryCodes} style={{ marginRight: '0.5rem' }}>
                          {authUser.recoveryCodesConfigured ? 'Regenerate recovery codes' : 'Generate recovery codes'}
                        </button>
                      )}
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
                {recoveryCodesError && <div className="error">{recoveryCodesError}</div>}
                {recoveryCodesStatus && <div className="status">{recoveryCodesStatus}</div>}
                {recoveryCodes.length > 0 && (
                  <div className="form compact" style={{ marginTop: '0.75rem' }}>
                    <div className="field">
                      <label>Recovery codes</label>
                      <div className="placeholder">
                        Generated {recoveryCodesGeneratedAt ? formatTime(recoveryCodesGeneratedAt) : 'just now'}. Store these now; they are only shown once.
                      </div>
                      <div style={{ display: 'grid', gap: '0.35rem', marginTop: '0.5rem' }}>
                        {recoveryCodes.map((code) => (
                          <code key={code}>{code}</code>
                        ))}
                      </div>
                    </div>
                    <div className="field actions">
                      <button className="button ghost" onClick={doDownloadRecoveryCodes}>
                        Download codes
                      </button>
                    </div>
                  </div>
                )}
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
                <div className="settings-title">Artifact Trust</div>
                {!canViewArtifactTrust ? (
                  <div className="placeholder">Operator role required to view artifact trust settings.</div>
                ) : (
                  <>
                    <div className="inline-row">
                      <button className="button ghost" onClick={() => loadArtifactTrustPolicy()}>
                        Refresh policy
                      </button>
                      <button className="button ghost" onClick={() => loadTrustedSigningKeys()}>
                        Refresh keys
                      </button>
                      {canManageArtifactTrust && (
                        <button className="button ghost" onClick={handleOpenCreateTrustedSigningKey}>
                          New trusted key
                        </button>
                      )}
                    </div>
                    {artifactTrustError && <div className="error">{artifactTrustError}</div>}
                    {artifactTrustStatus && <div className="status">{artifactTrustStatus}</div>}
                    <div className="form compact">
                      <div className="field">
                        <label>Verification Mode</label>
                        <select
                          value={artifactTrustPolicy.verificationMode}
                          disabled={!canManageArtifactTrust}
                          onChange={(e) => setArtifactTrustPolicyState((prev) => ({ ...prev, verificationMode: e.target.value }))}
                        >
                          <option value="allow_unsigned">allow_unsigned</option>
                          <option value="warn_unsigned">warn_unsigned</option>
                          <option value="require_verified">require_verified</option>
                        </select>
                      </div>
                      <div className="field">
                        <label>Allowed Signature Types</label>
                        <div className="inline-row">
                          {signatureTypeOptions.map((option) => (
                            <label key={option.id} className="chip">
                              <input
                                type="checkbox"
                                disabled={!canManageArtifactTrust}
                                checked={artifactTrustPolicy.allowedSignatureTypes.includes(option.id)}
                                onChange={() =>
                                  setArtifactTrustPolicyState((prev) => ({
                                    ...prev,
                                    allowedSignatureTypes: prev.allowedSignatureTypes.includes(option.id)
                                      ? prev.allowedSignatureTypes.filter((item) => item !== option.id)
                                      : [...prev.allowedSignatureTypes, option.id],
                                  }))}
                              />
                              {option.label}
                            </label>
                          ))}
                        </div>
                      </div>
                      <div className="field full">
                        <label>Allowed Signing Keys</label>
                        <div className="inline-row">
                          {activeTrustedSigningKeys.length === 0 && <span className="placeholder">No active trusted keys.</span>}
                          {activeTrustedSigningKeys.map((key) => (
                            <label key={key.keyId} className="chip">
                              <input
                                type="checkbox"
                                disabled={!canManageArtifactTrust}
                                checked={artifactTrustPolicy.allowedSigningKeyIds.includes(key.keyId)}
                                onChange={() =>
                                  setArtifactTrustPolicyState((prev) => ({
                                    ...prev,
                                    allowedSigningKeyIds: prev.allowedSigningKeyIds.includes(key.keyId)
                                      ? prev.allowedSigningKeyIds.filter((item) => item !== key.keyId)
                                      : [...prev.allowedSigningKeyIds, key.keyId],
                                  }))}
                              />
                              {key.displayName || key.keyId}
                            </label>
                          ))}
                        </div>
                      </div>
                      <div className="field actions">
                        <button className="button ghost" onClick={handleSaveArtifactTrustPolicy} disabled={!canManageArtifactTrust}>
                          Save policy
                        </button>
                        <span className="detail-note">
                          Updated {formatTime(artifactTrustPolicy.updatedAt)} by {artifactTrustPolicy.updatedByUserId || '—'}
                        </span>
                      </div>
                    </div>
                    <div className="table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>Name</th>
                            <th>Key ID</th>
                            <th>Algorithm</th>
                            <th>Status</th>
                            <th>Notes</th>
                            <th>Actions</th>
                          </tr>
                        </thead>
                        <tbody>
                          {trustedSigningKeys.map((key) => (
                            <tr key={key.keyId}>
                              <td>{key.displayName || '—'}</td>
                              <td className="mono">{key.keyId}</td>
                              <td>{key.algorithm}</td>
                              <td>
                                <span className={`pill ${String(key.state || '').toLowerCase() === 'retired' ? 'legacy' : 'verified'}`}>
                                  {key.state || 'active'}
                                </span>
                              </td>
                              <td>{key.notes || '—'}</td>
                              <td>
                                <div className="inline-row">
                                  <button
                                    className="button ghost"
                                    onClick={() => handleStartEditTrustedSigningKey(key)}
                                    disabled={!canManageArtifactTrust}
                                  >
                                    Edit
                                  </button>
                                  <button
                                    className="button ghost"
                                    onClick={() => handleRetireTrustedSigningKey(key)}
                                    disabled={!canManageArtifactTrust || String(key.state || '').toLowerCase() === 'retired'}
                                  >
                                    Retire
                                  </button>
                                </div>
                              </td>
                            </tr>
                          ))}
                          {trustedSigningKeys.length === 0 && (
                            <tr>
                              <td colSpan={6}>{trustedSigningKeysLoading ? 'Loading trusted signing keys...' : 'No trusted signing keys.'}</td>
                            </tr>
                          )}
                        </tbody>
                      </table>
                    </div>
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
    </>
  )
}
