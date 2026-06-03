export default function TrustOverrideModal({
  activeTrustedSigningKeys,
  artifactAllowedByTrustPolicy,
  artifactSignerSummary,
  artifactTrustPolicy,
  canManageDesiredState,
  closeTrustOverrideEditor,
  saveTrustOverrideEditor,
  setTrustOverrideError,
  setTrustOverrideForm,
  signatureTypeOptions,
  trustOverrideEditor,
  trustOverrideEditorRow,
  trustOverrideEffectivePolicy,
  trustOverrideError,
  trustOverrideForm,
  trustOverrideModes,
  trustOverrideSelectedArtifact,
  trustPolicyStrictness,
  verificationModeLabel,
  verificationPillLabel,
}) {
  return (
    <>
      {canManageDesiredState && trustOverrideEditor && trustOverrideEditorRow && (
        <div className="modal-backdrop" onClick={closeTrustOverrideEditor}>
          <div className="modal trust-override-modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Component Trust Override</h3>
              <button className="button ghost" onClick={closeTrustOverrideEditor}>
                Close
              </button>
            </div>
            <div className="detail-note">
              Component: <strong>{trustOverrideEditorRow.key || 'unnamed component'}</strong>
            </div>
            <div className="form">
              <div>
                <label>Verification Mode</label>
                <select
                  value={trustOverrideForm.verificationMode}
                  onChange={(e) => {
                    setTrustOverrideForm((prev) => ({ ...prev, verificationMode: e.target.value }))
                    setTrustOverrideError('')
                  }}
                >
                  {trustOverrideModes.map((mode) => {
                    const disabled = mode.id !== 'inherit' &&
                      trustPolicyStrictness(mode.id) < trustPolicyStrictness(artifactTrustPolicy.verificationMode)
                    return (
                      <option key={mode.id} value={mode.id} disabled={disabled}>
                        {mode.label}
                      </option>
                    )
                  })}
                </select>
              </div>
              <div className="full">
                <label>Allowed Signature Types</label>
                <div className="trust-option-grid">
                  {signatureTypeOptions.map((option) => (
                    <label key={option.id} className="inline-toggle">
                      <input
                        type="checkbox"
                        checked={trustOverrideForm.allowedSignatureTypes.includes(option.id)}
                        onChange={() => {
                          setTrustOverrideForm((prev) => ({
                            ...prev,
                            allowedSignatureTypes: prev.allowedSignatureTypes.includes(option.id)
                              ? prev.allowedSignatureTypes.filter((item) => item !== option.id)
                              : [...prev.allowedSignatureTypes, option.id],
                          }))
                          setTrustOverrideError('')
                        }}
                      />
                      {option.label}
                    </label>
                  ))}
                </div>
              </div>
              <div className="full">
                <label>Allowed Signing Keys</label>
                <div className="trust-option-grid">
                  {activeTrustedSigningKeys.length === 0 && <span className="placeholder">No active trusted keys loaded.</span>}
                  {activeTrustedSigningKeys.map((key) => (
                    <label key={key.keyId} className="inline-toggle">
                      <input
                        type="checkbox"
                        checked={trustOverrideForm.allowedSigningKeyIds.includes(key.keyId)}
                        onChange={() => {
                          setTrustOverrideForm((prev) => ({
                            ...prev,
                            allowedSigningKeyIds: prev.allowedSigningKeyIds.includes(key.keyId)
                              ? prev.allowedSigningKeyIds.filter((item) => item !== key.keyId)
                              : [...prev.allowedSigningKeyIds, key.keyId],
                          }))
                          setTrustOverrideError('')
                        }}
                      />
                      <span title={key.keyId}>{key.displayName || key.keyId}</span>
                    </label>
                  ))}
                </div>
              </div>
              <div className="full artifact-summary">
                <div>Effective policy: <strong>{verificationModeLabel(trustOverrideEffectivePolicy.verificationMode)}</strong></div>
                {trustOverrideEffectivePolicy.allowedSignatureTypes.length > 0 && (
                  <div className="detail-note">Types: {trustOverrideEffectivePolicy.allowedSignatureTypes.join(', ')}</div>
                )}
                {trustOverrideEffectivePolicy.allowedSigningKeyIds.length > 0 && (
                  <div className="detail-note">Keys: {trustOverrideEffectivePolicy.allowedSigningKeyIds.join(', ')}</div>
                )}
                {trustOverrideSelectedArtifact && (
                  <div className="detail-note">
                    Selected artifact: {verificationPillLabel(trustOverrideSelectedArtifact)} ({artifactSignerSummary(trustOverrideSelectedArtifact)})
                    {!artifactAllowedByTrustPolicy(trustOverrideSelectedArtifact, trustOverrideEffectivePolicy) &&
                      ' — blocked by effective trust policy'}
                  </div>
                )}
              </div>
              {trustOverrideError && <div className="error full">{trustOverrideError}</div>}
              <div className="full inline-row">
                <button
                  className="button ghost"
                  type="button"
                  onClick={() => {
                    setTrustOverrideForm({
                      verificationMode: 'inherit',
                      allowedSigningKeyIds: [],
                      allowedSignatureTypes: [],
                    })
                    setTrustOverrideError('')
                  }}
                >
                  Reset to inherited
                </button>
                <button className="button" type="button" onClick={saveTrustOverrideEditor}>
                  Apply trust override
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
