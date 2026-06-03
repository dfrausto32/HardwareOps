export default function EnrollmentProfileEditModal({
  canManagePendingEnrollments,
  editingEnrollmentProfileId,
  enrollmentProfileEditForm,
  handleCancelEditEnrollmentProfile,
  handleUpdateEnrollmentProfile,
  setEnrollmentProfileEditForm,
}) {
  return (
    <>
      {canManagePendingEnrollments && editingEnrollmentProfileId && (
        <div className="modal-backdrop" onClick={handleCancelEditEnrollmentProfile}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Edit Enrollment Profile</h3>
              <button className="button ghost" onClick={handleCancelEditEnrollmentProfile}>
                Close
              </button>
            </div>
            <div className="form compact two-column">
              <div className="field">
                <label>Name</label>
                <input
                  value={enrollmentProfileEditForm.name}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, name: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Max Uses (0 = unlimited)</label>
                <input
                  value={enrollmentProfileEditForm.maxUses}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, maxUses: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Approval Delay (sec)</label>
                <input
                  type="number"
                  min="0"
                  value={enrollmentProfileEditForm.approvalDelaySec}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, approvalDelaySec: e.target.value }))}
                />
              </div>
              <div className="field">
                <label className="chip">
                  <input
                    type="checkbox"
                    checked={enrollmentProfileEditForm.allowUnsignedHardwareIdentity}
                    onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, allowUnsignedHardwareIdentity: e.target.checked }))}
                  />
                  Allow unsigned hardware identity
                </label>
              </div>
              <div className="field">
                <label className="chip">
                  <input
                    type="checkbox"
                    checked={enrollmentProfileEditForm.challengeEnabled}
                    onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, challengeEnabled: e.target.checked }))}
                  />
                  Challenge enabled
                </label>
              </div>
              <div className="field">
                <label>Challenge Secret (leave blank to keep current)</label>
                <input
                  value={enrollmentProfileEditForm.challengeSecret}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, challengeSecret: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Challenge Hint</label>
                <input
                  value={enrollmentProfileEditForm.challengeHint}
                  onChange={(e) => setEnrollmentProfileEditForm((prev) => ({ ...prev, challengeHint: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Default Labels</label>
                <div className="key-value-list">
                  {enrollmentProfileEditForm.defaultLabelRows.map((row, idx) => (
                    <div key={`profile-edit-label-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...enrollmentProfileEditForm.defaultLabelRows]
                          next[idx] = { ...row, key: e.target.value }
                          setEnrollmentProfileEditForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...enrollmentProfileEditForm.defaultLabelRows]
                          next[idx] = { ...row, value: e.target.value }
                          setEnrollmentProfileEditForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = enrollmentProfileEditForm.defaultLabelRows.filter((_, ridx) => ridx !== idx)
                          setEnrollmentProfileEditForm((prev) => ({ ...prev, defaultLabelRows: next.length > 0 ? next : [{ key: '', value: '' }] }))
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setEnrollmentProfileEditForm((prev) => ({
                      ...prev,
                      defaultLabelRows: [...prev.defaultLabelRows, { key: '', value: '' }],
                    }))}
                  >
                    Add Label
                  </button>
                </div>
              </div>
              <div className="field actions">
                <button className="button ghost" onClick={handleUpdateEnrollmentProfile}>
                  Save profile
                </button>
                <button className="button ghost" onClick={handleCancelEditEnrollmentProfile}>
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
