export default function ArtifactPickerModal({
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
}) {
  return (
    <>
      {canManageDesiredState && artifactModalOpen && (
        <div
          className="modal-backdrop"
          onClick={() => {
            setArtifactModalOpen(false)
            setArtifactPickerTarget(null)
          }}
        >
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Select Artifact</h3>
              <button
                className="button ghost"
                onClick={() => {
                  setArtifactModalOpen(false)
                  setArtifactPickerTarget(null)
                }}
              >
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
                    <th>Trust</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {artifactModalGroups.map((group) => (
                    group.versions.map((a, idx) => {
                      const selectable = artifactAllowedByTrustPolicy(a, artifactPickerPolicy)
                      return (
                      <tr key={a.artifactId} className={idx === 0 ? 'artifact-group-start' : ''}>
                        <td>{idx === 0 ? group.name : ''}</td>
                        <td className="mono">{a.artifactId}</td>
                        <td>{a.version}</td>
                        <td>
                          <span className={`pill ${normalizeVerificationStatus(a.verificationStatus, a)}`}>
                            {verificationPillLabel(a)}
                          </span>
                          <div className="detail-note">{artifactSignerSummary(a)}</div>
                        </td>
                        <td>
                          <button
                            className="button ghost"
                            disabled={!selectable}
                            onClick={() => {
                              if (artifactPickerTarget?.scope === 'group') {
                                updateGroupComponent(artifactPickerTarget.index, {
                                  artifactId: a.artifactId,
                                  desiredVersion: a.version,
                                  artifactType: normalizeArtifactType(a.type),
                                })
                              } else if (artifactPickerTarget?.scope === 'groupBulk') {
                                updateGroupMultiDesiredComponent(artifactPickerTarget.index, {
                                  artifactId: a.artifactId,
                                  desiredVersion: a.version,
                                  artifactType: normalizeArtifactType(a.type),
                                })
                              } else if (artifactPickerTarget?.scope === 'device') {
                                updateDeviceComponent(artifactPickerTarget.index, {
                                  artifactId: a.artifactId,
                                  desiredVersion: a.version,
                                  artifactType: normalizeArtifactType(a.type),
                                })
                              }
                              setArtifactModalOpen(false)
                              setArtifactPickerTarget(null)
                            }}
                          >
                            {selectable ? 'Select' : 'Blocked'}
                          </button>
                        </td>
                      </tr>
                    )})
                  ))}
                  {artifactModalGroups.length === 0 && (
                    <tr>
                      <td colSpan={5}>No active artifacts match this component type.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
