export default function ArtifactUploadModal({
  activeTrustedSigningKeys,
  artifactTrustError,
  artifactTrustPolicy,
  artifactUploadOpen,
  canManageArtifacts,
  handleUpload,
  setArtifactUploadOpen,
  signatureTypeOptions,
  uploadStatus,
}) {
  return (
    <>
      {canManageArtifacts && artifactUploadOpen && (
        <div className="modal-backdrop" onClick={() => setArtifactUploadOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Upload Artifact</h3>
              <button className="button ghost" onClick={() => setArtifactUploadOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleUpload}>
              <div>
                <label>Name</label>
                <input name="name" required placeholder="agent" />
              </div>
              <div>
                <label>Version</label>
                <input name="version" required placeholder="1.0.0" />
              </div>
              <div className="full">
                <label>Bundle</label>
                <input name="file" type="file" required />
              </div>
              <div className="full">
                <label>Detached Signature (optional)</label>
                <input name="signatureFile" type="file" accept=".sig,.txt,.b64,text/plain" />
                <div className="hint">
                  Upload the detached base64 signature file for this bundle. Leave blank for unsigned upload when policy allows it.
                </div>
              </div>
              <div>
                <label>Signature Type</label>
                <select name="signatureType" defaultValue="ed25519">
                  {signatureTypeOptions.map((option) => (
                    <option key={option.id} value={option.id}>{option.label}</option>
                  ))}
                </select>
              </div>
              <div className="full">
                <label>Signing Key ID (optional)</label>
                <input name="signatureKeyId" placeholder="sha256:..." list="trusted-signing-key-ids" />
                <datalist id="trusted-signing-key-ids">
                  {activeTrustedSigningKeys.map((key) => (
                    <option key={key.keyId} value={key.keyId}>
                      {key.displayName || key.keyId}
                    </option>
                  ))}
                </datalist>
                <div className="hint">
                  Use the trusted public key ID that matches the detached signature. This is recorded with the artifact and used by signature policy.
                </div>
              </div>
              <div className="full">
                <div className="hint">
                  Current trust policy: <strong>{artifactTrustPolicy.verificationMode || 'warn_unsigned'}</strong>.
                  {artifactTrustPolicy.verificationMode === 'require_verified'
                    ? ' Signed upload is required.'
                    : ' Unsigned upload is allowed by policy.'}
                </div>
                {artifactTrustPolicy.allowedSignatureTypes?.length > 0 && (
                  <div className="hint">
                    Allowed signature types: {artifactTrustPolicy.allowedSignatureTypes.join(', ')}
                  </div>
                )}
                {artifactTrustPolicy.allowedSigningKeyIds?.length > 0 && (
                  <div className="hint">
                    Allowed signing keys: {artifactTrustPolicy.allowedSigningKeyIds.join(', ')}
                  </div>
                )}
                {artifactTrustError && <div className="error">{artifactTrustError}</div>}
              </div>
              <div className="full inline-row">
                <button className="button" type="submit">Upload</button>
                {uploadStatus && <span className="status">{uploadStatus}</span>}
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}
