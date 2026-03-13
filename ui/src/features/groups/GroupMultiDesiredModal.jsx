export default function GroupMultiDesiredModal({
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
}) {
  return (
    <>
      {canManageDesiredState && groupMultiDesiredOpen && (
        <div className="modal-backdrop" onClick={() => setGroupMultiDesiredOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Set Desired for Selected Groups</h3>
              <button className="button ghost" onClick={() => setGroupMultiDesiredOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleApplyGroupMultiDesired}>
              <label>Selected Groups</label>
              <input value={String(selectedGroupIds.length)} readOnly />
              <label>Components</label>
              <div className="component-editor">
                {groupMultiDesiredForm.components.map((row, idx) => {
                  const locked = Boolean(row.locked)
                  const type = normalizeArtifactType(row.artifactType)
                  const trackingPreview = getTrackingPreview(row, 'group')
                  const trackingEnabled = String(row.autoTrackMode || 'inherit') !== 'disabled'
                  const groupsForType = filterArtifactGroupsByType(activeArtifactGroups, type)
                  const effectiveTrustPolicy = resolveEffectiveTrustPolicy(row.policy, artifactTrustPolicy)
                  const allowedGroupsForType = filterArtifactGroupsByTrustPolicy(groupsForType, effectiveTrustPolicy)
                  const allGroupsForType = filterArtifactGroupsByType(artifactGroups, type)
                  const selected = artifacts.find((a) => a.artifactId === row.artifactId)
                  const selectedGroup = selected
                    ? (groupsForType.find((g) => g.name === selected.name && normalizeArtifactType(g.type) === normalizeArtifactType(selected.type)) ||
                      allGroupsForType.find((g) => g.name === selected.name && normalizeArtifactType(g.type) === normalizeArtifactType(selected.type)))
                    : null
                  return (
                    <div className="component-row" key={row.id || `${row.key}-${idx}`}>
                      <div className="inline-row">
                        <input
                          value={row.key}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { key: e.target.value })}
                          placeholder="component key (e.g. app:customer)"
                          disabled={locked}
                        />
                        <input
                          list="artifact-types"
                          value={row.artifactType}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { artifactType: e.target.value })}
                          placeholder="artifact type"
                          disabled={locked}
                        />
                        <select
                          value={row.autoTrackMode || 'inherit'}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { autoTrackMode: e.target.value })}
                          disabled={locked}
                        >
                          {groupTrackingModes.map((mode) => (
                            <option key={mode.id} value={mode.id}>{mode.label}</option>
                          ))}
                        </select>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => openTrustOverrideEditor('groupBulk', idx)}
                          disabled={locked}
                        >
                          Trust
                        </button>
                        <label className="inline-toggle">
                          <input
                            type="checkbox"
                            checked={locked}
                            onChange={(e) => updateGroupMultiDesiredComponent(idx, { locked: e.target.checked })}
                          />
                          Lock
                        </label>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => removeGroupMultiDesiredComponent(idx)}
                          disabled={locked || groupMultiDesiredForm.components.length <= 1}
                        >
                          Remove
                        </button>
                      </div>
                      {trackingEnabled && (
                        <div className="inline-row">
                          <input
                            value={row.trackingName || ''}
                            onChange={(e) => updateGroupMultiDesiredComponent(idx, { trackingName: e.target.value })}
                            placeholder="tracked artifact name"
                            disabled={locked}
                          />
                        </div>
                      )}
                      <div className="inline-row">
                        <select
                          value={selected?.name || ''}
                          onChange={(e) => {
                            const name = e.target.value
                            const group = allowedGroupsForType.find((g) => g.name === name)
                            if (!group) {
                              updateGroupMultiDesiredComponent(idx, { artifactId: '', desiredVersion: '' })
                            } else {
                              const pick = group.versions[group.versions.length - 1]
                              updateGroupMultiDesiredComponent(idx, {
                                artifactId: pick.artifactId,
                                desiredVersion: pick.version,
                                artifactType: normalizeArtifactType(pick.type),
                              })
                            }
                          }}
                          disabled={locked || trackingEnabled}
                        >
                          <option value="">Select artifact</option>
                          {allowedGroupsForType.map((group) => (
                            <option key={group.name} value={group.name}>{group.name}</option>
                          ))}
                        </select>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => openArtifactPicker('groupBulk', idx)}
                          disabled={locked || trackingEnabled}
                        >
                          Browse
                        </button>
                        <select
                          value={selected?.version || ''}
                          onChange={(e) => {
                            const version = e.target.value
                            const pick = selectedGroup?.versions.find((v) => v.version === version)
                            if (pick) {
                              updateGroupMultiDesiredComponent(idx, {
                                artifactId: pick.artifactId,
                                desiredVersion: pick.version,
                                artifactType: normalizeArtifactType(pick.type),
                              })
                            }
                          }}
                          disabled={locked || trackingEnabled}
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
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { desiredVersion: e.target.value })}
                          placeholder="desired version"
                          disabled={locked || trackingEnabled}
                        />
                        <input
                          value={row.desiredConfigRev}
                          onChange={(e) => updateGroupMultiDesiredComponent(idx, { desiredConfigRev: e.target.value })}
                          placeholder="config rev"
                          disabled={locked || trackingEnabled}
                        />
                      </div>
                      <div className="detail-note">Tracking: {trackingPreview.message}</div>
                      {trackingPreview.latestEligible && (
                        <div className="detail-note">
                          Latest eligible: {trackingPreview.latestEligible.version} ({artifactSignerSummary(trackingPreview.latestEligible)})
                        </div>
                      )}
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
                <button className="button ghost" type="button" onClick={addGroupMultiDesiredComponent}>
                  Add component
                </button>
              </div>
              <label>Check-in Interval (sec)</label>
              <input
                value={groupMultiDesiredForm.checkinIntervalSec}
                onChange={(e) => setGroupMultiDesiredForm({ ...groupMultiDesiredForm, checkinIntervalSec: e.target.value })}
                placeholder="30"
              />
              <div className="inline-row">
                <button className="button" type="submit">Apply to Selected</button>
                {groupMultiDesiredStatus && <span className="status">{groupMultiDesiredStatus}</span>}
              </div>
            </form>
            {groupMultiDesiredError && <div className="error">{groupMultiDesiredError}</div>}
          </div>
        </div>
      )}
    </>
  )
}
