export default function DeviceDrawer({
  activeArtifactGroups,
  addDeviceComponent,
  artifactAllowedByTrustPolicy,
  artifactGroups,
  artifactSignerSummary,
  artifactTrustPolicy,
  artifacts,
  autoTrackModes,
  buildComponentRows,
  canDecommissionDevices,
  canManageDesiredState,
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
}) {
  return (
    <>
      {deviceDrawerOpen && (
        <div className="drawer-backdrop" onClick={() => setDeviceDrawerOpen(false)}>
          <div
            className="drawer"
            style={{ width: `min(${drawerWidth}px, 90vw)` }}
            onClick={(e) => e.stopPropagation()}
          >
            <div
              className="drawer-handle"
              onPointerDown={(event) => {
                drawerResizingRef.current = true
                event.preventDefault()
              }}
            />
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

                {deviceDetail.current?.components && Object.keys(deviceDetail.current.components).some((key) => key !== 'app_bundle') && (
                  <div className="component-table">
                    <h4>Components</h4>
                    <table>
                      <thead>
                        <tr>
                          <th>Component</th>
                          <th>Current</th>
                          <th>Last Apply</th>
                          <th>Artifact</th>
                        </tr>
                      </thead>
                      <tbody>
                        {Object.entries(deviceDetail.current.components)
                          .filter(([key]) => key !== 'app_bundle')
                          .map(([key, comp]) => (
                          <tr key={key}>
                            <td>{key}</td>
                            <td>{comp.currentVersion || '—'}</td>
                            <td>
                              <span className={`pill ${comp.lastApplyStatus === 'error' ? 'error' : 'success'}`}>
                                {comp.lastApplyStatus || '—'}
                              </span>
                              {comp.lastApplyAt && <div className="detail-note">{comp.lastApplyAt}</div>}
                            </td>
                            <td>{comp.lastApplyArtifactId || '—'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                <form
                  className="form"
                  onSubmit={(e) => {
                    handleDesiredDevice(e)
                  }}
                >
                  <h3>Desired State</h3>
                  {desiredError && <div className="error">{desiredError}</div>}
                  {!canManageDesiredState && (
                    <div className="hint">Operator role required to edit desired state.</div>
                  )}
                  <fieldset disabled={!canManageDesiredState} style={{ border: 0, margin: 0, padding: 0, minWidth: 0 }}>
                    <label>Device ID</label>
                    <input value={deviceForm.deviceId} readOnly />
                    <label>Components</label>
                    <div className="component-editor">
                      {deviceForm.components.map((row, idx) => {
                        const locked = Boolean(row.locked)
                        const type = normalizeArtifactType(row.artifactType)
                        const groupsForType = filterArtifactGroupsByType(activeArtifactGroups, type)
                        const effectiveTrustPolicy = resolveEffectiveTrustPolicy(row.policy, artifactTrustPolicy)
                        const allowedGroupsForType = filterArtifactGroupsByTrustPolicy(groupsForType, effectiveTrustPolicy)
                        const allGroupsForType = filterArtifactGroupsByType(artifactGroups, type)
                        const selected = artifacts.find((a) => a.artifactId === row.artifactId)
                        const selectedGroup = selected
                          ? (groupsForType.find((g) => g.name === selected.name) ||
                            allGroupsForType.find((g) => g.name === selected.name))
                          : null
                        return (
                          <div className="component-row" key={row.id || `${row.key}-${idx}`}>
                            <div className="inline-row">
                              <input
                                value={row.key}
                                onChange={(e) => updateDeviceComponent(idx, { key: e.target.value })}
                                placeholder="component key (e.g. app:customer)"
                                disabled={locked}
                              />
                              <input
                                list="artifact-types"
                                value={row.artifactType}
                                onChange={(e) => updateDeviceComponent(idx, { artifactType: e.target.value })}
                                placeholder="artifact type"
                                disabled={locked}
                              />
                              <select
                                value={row.autoTrackMode || 'inherit'}
                                onChange={(e) => updateDeviceComponent(idx, { autoTrackMode: e.target.value })}
                                disabled={locked}
                              >
                                {autoTrackModes.map((mode) => (
                                  <option key={mode.id} value={mode.id}>{`Auto ${mode.label}`}</option>
                                ))}
                              </select>
                              <button
                                className="button ghost"
                                type="button"
                                onClick={() => openTrustOverrideEditor('device', idx)}
                                disabled={locked}
                              >
                                Trust
                              </button>
                              <label className="inline-toggle">
                                <input
                                  type="checkbox"
                                  checked={locked}
                                  onChange={(e) => updateDeviceComponent(idx, { locked: e.target.checked })}
                                />
                                Lock
                              </label>
                              <button
                                className="button ghost"
                                type="button"
                                onClick={() => removeDeviceComponent(idx)}
                                disabled={locked || deviceForm.components.length <= 1}
                              >
                                Remove
                              </button>
                            </div>
                            <div className="inline-row">
                              <select
                                value={selected?.name || ''}
                                onChange={(e) => {
                                  const name = e.target.value
                                  const group = allowedGroupsForType.find((g) => g.name === name)
                                  if (!group) {
                                    updateDeviceComponent(idx, { artifactId: '', desiredVersion: '' })
                                  } else {
                                    const pick = group.versions[group.versions.length - 1]
                                    updateDeviceComponent(idx, {
                                      artifactId: pick.artifactId,
                                      desiredVersion: pick.version,
                                      artifactType: normalizeArtifactType(pick.type),
                                    })
                                  }
                                }}
                                disabled={locked}
                              >
                                <option value="">Select artifact</option>
                                {allowedGroupsForType.map((group) => (
                                  <option key={group.name} value={group.name}>{group.name}</option>
                                ))}
                              </select>
                              <button
                                className="button ghost"
                                type="button"
                                onClick={() => openArtifactPicker('device', idx)}
                                disabled={locked}
                              >
                                Browse
                              </button>
                              <select
                                value={selected?.version || ''}
                                onChange={(e) => {
                                  const version = e.target.value
                                  const pick = selectedGroup?.versions.find((v) => v.version === version)
                                  if (pick) {
                                    updateDeviceComponent(idx, {
                                      artifactId: pick.artifactId,
                                      desiredVersion: pick.version,
                                      artifactType: normalizeArtifactType(pick.type),
                                    })
                                  }
                                }}
                                disabled={locked}
                              >
                                <option value="">Select version</option>
                                {(selectedGroup?.versions || []).filter((artifact) => artifactAllowedByTrustPolicy(artifact, effectiveTrustPolicy)).map((artifact) => (
                                  <option key={artifact.artifactId} value={artifact.version}>
                                    {artifact.version}
                                  </option>
                                ))}
                              </select>
                            </div>
                            <div className="inline-row">
                              <input value={row.artifactId} readOnly placeholder="artifact uuid" />
                              <input
                                value={row.desiredVersion}
                                onChange={(e) => updateDeviceComponent(idx, { desiredVersion: e.target.value })}
                                placeholder="desired version"
                                disabled={locked}
                              />
                              <input
                                value={row.desiredConfigRev}
                                onChange={(e) => updateDeviceComponent(idx, { desiredConfigRev: e.target.value })}
                                placeholder="config rev"
                                disabled={locked}
                              />
                            </div>
                            <div className="detail-note">Trust policy: {trustPolicySummary(row.policy, artifactTrustPolicy)}</div>
                            {selected && (
                              <div className="detail-note">
                                Trust: {verificationPillLabel(selected)} ({artifactSignerSummary(selected)})
                                {!artifactAllowedByTrustPolicy(selected, effectiveTrustPolicy) && ' — blocked by effective trust policy'}
                              </div>
                            )}
                          </div>
                        )
                      })}
                      <button className="button ghost" type="button" onClick={addDeviceComponent}>
                        Add component
                      </button>
                    </div>
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
                            checkinIntervalSec: selectedDesired?.checkinIntervalSec ? String(selectedDesired.checkinIntervalSec) : '',
                            components: buildComponentRows(
                              selectedDesired?.components || {},
                              {
                                artifactId: selectedDesired?.artifactId || '',
                                desiredVersion: selectedDesired?.desiredVersion || deviceDetail.current?.softwareVersion || '',
                                desiredConfigRev: selectedDesired?.desiredConfigRev || deviceDetail.current?.configRev || '',
                              },
                              deviceDetail?.current,
                            ),
                          })
                        }}
                      >
                        Reset
                      </button>
                    </div>
                  </fieldset>
                </form>

                <div className="inline-row">
                  <button className="button ghost" onClick={() => downloadLogs(selectedDeviceId)}>
                    Download Logs
                  </button>
                  {canDecommissionDevices && (
                    <button
                      className="button ghost"
                      onClick={() => handleDeleteDevice(selectedDeviceId)}
                    >
                      Decommission Device
                    </button>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </>
  )
}
