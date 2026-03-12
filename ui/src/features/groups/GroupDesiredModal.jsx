export default function GroupDesiredModal({
  activeArtifactGroups,
  addGroupComponent,
  artifactAllowedByTrustPolicy,
  artifactGroups,
  artifactSignerSummary,
  artifactTrustPolicy,
  artifacts,
  autoTrackModes,
  canManageDesiredState,
  filterArtifactGroupsByTrustPolicy,
  filterArtifactGroupsByType,
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
}) {
  return (
    <>
      {canManageDesiredState && groupDesiredOpen && (
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
              <label>Components</label>
              <div className="component-editor">
                {groupDesiredForm.components.map((row, idx) => {
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
                          onChange={(e) => updateGroupComponent(idx, { key: e.target.value })}
                          placeholder="component key (e.g. app:customer)"
                          disabled={locked}
                        />
                        <input
                          list="artifact-types"
                          value={row.artifactType}
                          onChange={(e) => updateGroupComponent(idx, { artifactType: e.target.value })}
                          placeholder="artifact type"
                          disabled={locked}
                        />
                        <select
                          value={row.autoTrackMode || 'inherit'}
                          onChange={(e) => updateGroupComponent(idx, { autoTrackMode: e.target.value })}
                          disabled={locked}
                        >
                          {autoTrackModes.map((mode) => (
                            <option key={mode.id} value={mode.id}>{`Auto ${mode.label}`}</option>
                          ))}
                        </select>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => openTrustOverrideEditor('group', idx)}
                          disabled={locked}
                        >
                          Trust
                        </button>
                        <label className="inline-toggle">
                          <input
                            type="checkbox"
                            checked={locked}
                            onChange={(e) => updateGroupComponent(idx, { locked: e.target.checked })}
                          />
                          Lock
                        </label>
                        <button
                          className="button ghost"
                          type="button"
                          onClick={() => removeGroupComponent(idx)}
                          disabled={locked || groupDesiredForm.components.length <= 1}
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
                              updateGroupComponent(idx, { artifactId: '', desiredVersion: '' })
                            } else {
                              const pick = group.versions[group.versions.length - 1]
                              updateGroupComponent(idx, {
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
                          onClick={() => openArtifactPicker('group', idx)}
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
                              updateGroupComponent(idx, {
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
                          onChange={(e) => updateGroupComponent(idx, { desiredVersion: e.target.value })}
                          placeholder="desired version"
                          disabled={locked}
                        />
                        <input
                          value={row.desiredConfigRev}
                          onChange={(e) => updateGroupComponent(idx, { desiredConfigRev: e.target.value })}
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
                <button className="button ghost" type="button" onClick={addGroupComponent}>
                  Add component
                </button>
              </div>
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
    </>
  )
}
