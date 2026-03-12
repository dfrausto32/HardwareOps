export default function BulkGroupManagementModal({
  canManageGroups,
  downloadGroupBulkRollback,
  downloadGroupBulkTemplate,
  formatSelector,
  groupBulkApplying,
  groupBulkCsv,
  groupBulkError,
  groupBulkOpen,
  groupBulkPreview,
  groupBulkRollbackCsv,
  groupBulkStatus,
  handleApplyGroupBulk,
  handleGroupBulkFileChange,
  handlePreviewGroupBulk,
  setGroupBulkCsv,
  setGroupBulkError,
  setGroupBulkOpen,
  setGroupBulkPreview,
  setGroupBulkRollbackCsv,
  setGroupBulkStatus,
}) {
  return (
    <>
      {canManageGroups && groupBulkOpen && (
        <div className="modal-backdrop" onClick={() => setGroupBulkOpen(false)}>
          <div className="modal bulk-group-modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>Bulk Group Management</h3>
              <button className="button ghost" onClick={() => setGroupBulkOpen(false)}>
                Close
              </button>
            </div>
            <div className="bulk-group-toolbar">
              <button className="button ghost" onClick={downloadGroupBulkTemplate}>
                Download template
              </button>
              <label className="button ghost bulk-group-file">
                Load CSV
                <input
                  type="file"
                  accept=".csv,text/csv"
                  onChange={handleGroupBulkFileChange}
                />
              </label>
              <button className="button ghost" onClick={handlePreviewGroupBulk}>
                Preview
              </button>
              <button className="button" onClick={handleApplyGroupBulk} disabled={groupBulkApplying}>
                {groupBulkApplying ? 'Applying…' : 'Apply'}
              </button>
              {groupBulkRollbackCsv && (
                <button className="button ghost" onClick={downloadGroupBulkRollback}>
                  Download rollback CSV
                </button>
              )}
            </div>
            <div className="hint">
              CSV columns: <code>action</code>, <code>groupId</code>, <code>name</code>, <code>region</code>, <code>role</code>,
              <code>site</code>, <code>selector_json</code>, or dynamic <code>selector.&lt;key&gt;</code> columns.
            </div>
            <div className="form">
              <div className="full">
                <label>CSV input</label>
                <textarea
                  className="bulk-group-input"
                  value={groupBulkCsv}
                  onChange={(e) => {
                    setGroupBulkCsv(e.target.value)
                    setGroupBulkPreview(null)
                    setGroupBulkRollbackCsv('')
                    setGroupBulkError('')
                    setGroupBulkStatus('')
                  }}
                  placeholder="action,groupId,name,region,role,site,selector_json"
                />
              </div>
            </div>
            {groupBulkError && <div className="error">{groupBulkError}</div>}
            {groupBulkStatus && <div className="status">{groupBulkStatus}</div>}
            {groupBulkPreview && (
              <>
                <div className="detail-note">
                  Rows: {groupBulkPreview.summary.totalRows} · valid {groupBulkPreview.summary.validRows} · errors {groupBulkPreview.summary.errorRows} ·
                  create {groupBulkPreview.summary.createRows} · update {groupBulkPreview.summary.updateRows} ·
                  delete {groupBulkPreview.summary.deleteRows} · no change {groupBulkPreview.summary.noChangeRows}
                </div>
                <div className="table-wrap bulk-group-table">
                  <table>
                    <thead>
                      <tr>
                        <th>Line</th>
                        <th>Action</th>
                        <th>Group ID</th>
                        <th>Name</th>
                        <th>Selector</th>
                        <th>Preview</th>
                        <th>Apply</th>
                      </tr>
                    </thead>
                    <tbody>
                      {groupBulkPreview.rows.map((row) => (
                        <tr key={row.rowId}>
                          <td>{row.line}</td>
                          <td>{row.action}</td>
                          <td className="mono">{row.groupId || '—'}</td>
                          <td>{row.name || '—'}</td>
                          <td><code>{row.action === 'upsert' ? formatSelector(row.selector) : '—'}</code></td>
                          <td>
                            {row.error ? (
                              <span className="status error-inline">{row.error}</span>
                            ) : (
                              <span className="pill">{row.outcome}</span>
                            )}
                          </td>
                          <td>
                            {row.applyStatus ? (
                              <span className={`pill ${row.applyStatus === 'applied' ? 'signed' : 'unsigned'}`}>
                                {row.applyStatus === 'applied' ? 'ok' : 'failed'}
                              </span>
                            ) : '—'}
                            {row.applyError && <div className="status error-inline">{row.applyError}</div>}
                          </td>
                        </tr>
                      ))}
                      {groupBulkPreview.rows.length === 0 && (
                        <tr>
                          <td colSpan={7}>No rows parsed.</td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </>
  )
}
