export default function EnrollmentProfileCreateModal({
  canManagePendingEnrollments,
  enrollmentProfileCreateOpen,
  enrollmentProfileForm,
  handleCreateEnrollmentProfile,
  setEnrollmentProfileCreateOpen,
  setEnrollmentProfileForm,
}) {
  return (
    <>
      {canManagePendingEnrollments && enrollmentProfileCreateOpen && (
        <div className="modal-backdrop" onClick={() => setEnrollmentProfileCreateOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Create Enrollment Profile</h3>
              <button className="button ghost" onClick={() => setEnrollmentProfileCreateOpen(false)}>
                Close
              </button>
            </div>
            <div className="form compact two-column">
              <div className="field">
                <label>Name</label>
                <input
                  value={enrollmentProfileForm.name}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, name: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Expiration Days</label>
                <input
                  value={enrollmentProfileForm.expiresInDays}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, expiresInDays: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Max Uses (0 = unlimited)</label>
                <input
                  value={enrollmentProfileForm.maxUses}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, maxUses: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Challenge Secret (optional)</label>
                <input
                  value={enrollmentProfileForm.challengeSecret}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, challengeSecret: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Challenge Hint (optional)</label>
                <input
                  value={enrollmentProfileForm.challengeHint}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, challengeHint: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Approval Delay (sec)</label>
                <input
                  type="number"
                  min="0"
                  value={enrollmentProfileForm.approvalDelaySec}
                  onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, approvalDelaySec: e.target.value }))}
                />
              </div>
              <div className="field">
                <label>Default Labels</label>
                <div className="key-value-list">
                  {enrollmentProfileForm.defaultLabelRows.map((row, idx) => (
                    <div key={`profile-create-label-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...enrollmentProfileForm.defaultLabelRows]
                          next[idx] = { ...row, key: e.target.value }
                          setEnrollmentProfileForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...enrollmentProfileForm.defaultLabelRows]
                          next[idx] = { ...row, value: e.target.value }
                          setEnrollmentProfileForm((prev) => ({ ...prev, defaultLabelRows: next }))
                        }}
                        placeholder="label value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = enrollmentProfileForm.defaultLabelRows.filter((_, ridx) => ridx !== idx)
                          setEnrollmentProfileForm((prev) => ({ ...prev, defaultLabelRows: next.length > 0 ? next : [{ key: '', value: '' }] }))
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setEnrollmentProfileForm((prev) => ({
                      ...prev,
                      defaultLabelRows: [...prev.defaultLabelRows, { key: '', value: '' }],
                    }))}
                  >
                    Add Label
                  </button>
                </div>
              </div>
              <div className="field">
                <label className="chip">
                  <input
                    type="checkbox"
                    checked={enrollmentProfileForm.allowUnsignedHardwareIdentity}
                    onChange={(e) => setEnrollmentProfileForm((prev) => ({ ...prev, allowUnsignedHardwareIdentity: e.target.checked }))}
                  />
                  Allow unsigned hardware identity
                </label>
              </div>
              <div className="field actions">
                <button className="button ghost" onClick={handleCreateEnrollmentProfile}>
                  Create profile
                </button>
                <button className="button ghost" onClick={() => setEnrollmentProfileCreateOpen(false)}>
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
