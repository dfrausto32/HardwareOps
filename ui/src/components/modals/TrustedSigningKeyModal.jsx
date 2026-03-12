export default function TrustedSigningKeyModal({
  canManageArtifactTrust,
  editingTrustedSigningKeyId,
  handleSaveTrustedSigningKey,
  setTrustedSigningKeyForm,
  setTrustedSigningKeyModalOpen,
  trustedSigningKeyForm,
  trustedSigningKeyModalOpen,
}) {
  return (
    <>
      {canManageArtifactTrust && trustedSigningKeyModalOpen && (
        <div className="modal-backdrop" onClick={() => setTrustedSigningKeyModalOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>{editingTrustedSigningKeyId ? 'Edit Trusted Signing Key' : 'New Trusted Signing Key'}</h3>
              <button className="button ghost" onClick={() => setTrustedSigningKeyModalOpen(false)}>
                Close
              </button>
            </div>
            <form className="form compact" onSubmit={handleSaveTrustedSigningKey}>
              <div className="field">
                <label>Display Name</label>
                <input
                  value={trustedSigningKeyForm.displayName}
                  onChange={(e) => setTrustedSigningKeyForm((prev) => ({ ...prev, displayName: e.target.value }))}
                  placeholder="Release signing key"
                />
              </div>
              <div className="field">
                <label>Algorithm</label>
                <select
                  value={trustedSigningKeyForm.algorithm}
                  disabled={Boolean(editingTrustedSigningKeyId)}
                  onChange={(e) => setTrustedSigningKeyForm((prev) => ({ ...prev, algorithm: e.target.value }))}
                >
                  <option value="ed25519">ed25519</option>
                  <option value="cosign">cosign</option>
                </select>
              </div>
              <div className="field full">
                <label>Public Key PEM</label>
                <textarea
                  rows={8}
                  value={trustedSigningKeyForm.publicKeyPem}
                  disabled={Boolean(editingTrustedSigningKeyId)}
                  onChange={(e) => setTrustedSigningKeyForm((prev) => ({ ...prev, publicKeyPem: e.target.value }))}
                  placeholder="-----BEGIN PUBLIC KEY-----"
                />
              </div>
              <div className="field full">
                <label>Notes</label>
                <textarea
                  rows={3}
                  value={trustedSigningKeyForm.notes}
                  onChange={(e) => setTrustedSigningKeyForm((prev) => ({ ...prev, notes: e.target.value }))}
                  placeholder="Rotation window, owner, provenance notes..."
                />
              </div>
              <div className="field actions">
                <button className="button ghost" type="submit">
                  {editingTrustedSigningKeyId ? 'Save changes' : 'Create key'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}
