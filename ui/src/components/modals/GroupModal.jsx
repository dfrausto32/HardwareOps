export default function GroupModal({
  canManageGroups,
  groupForm,
  groupModalOpen,
  groupsStatus,
  handleSaveGroup,
  setGroupForm,
  setGroupModalOpen,
}) {
  return (
    <>
      {canManageGroups && groupModalOpen && (
        <div className="modal-backdrop" onClick={() => setGroupModalOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="section-header">
              <h3>{groupForm.groupId ? 'Edit Group' : 'Create Group'}</h3>
              <button className="button ghost" onClick={() => setGroupModalOpen(false)}>
                Close
              </button>
            </div>
            <form className="form" onSubmit={handleSaveGroup}>
              <div>
                <label>Name</label>
                <input
                  value={groupForm.name}
                  onChange={(e) => setGroupForm({ ...groupForm, name: e.target.value })}
                  placeholder="canary"
                />
              </div>
              <div>
                <label>Region</label>
                <input
                  value={groupForm.region}
                  onChange={(e) => setGroupForm({ ...groupForm, region: e.target.value })}
                  placeholder="west"
                />
              </div>
              <div>
                <label>Role</label>
                <input
                  value={groupForm.role}
                  onChange={(e) => setGroupForm({ ...groupForm, role: e.target.value })}
                  placeholder="edge"
                />
              </div>
              <div>
                <label>Site</label>
                <input
                  value={groupForm.site}
                  onChange={(e) => setGroupForm({ ...groupForm, site: e.target.value })}
                  placeholder="lab-1"
                />
              </div>
              <div className="full">
                <label>Custom Labels</label>
                <div className="key-value-list">
                  {groupForm.custom.map((row, idx) => (
                    <div key={`custom-${idx}`} className="key-value-row">
                      <input
                        value={row.key}
                        onChange={(e) => {
                          const next = [...groupForm.custom]
                          next[idx] = { ...row, key: e.target.value }
                          setGroupForm({ ...groupForm, custom: next })
                        }}
                        placeholder="key"
                      />
                      <input
                        value={row.value}
                        onChange={(e) => {
                          const next = [...groupForm.custom]
                          next[idx] = { ...row, value: e.target.value }
                          setGroupForm({ ...groupForm, custom: next })
                        }}
                        placeholder="value"
                      />
                      <button
                        className="button ghost"
                        type="button"
                        onClick={() => {
                          const next = groupForm.custom.filter((_, cidx) => cidx !== idx)
                          setGroupForm({ ...groupForm, custom: next })
                        }}
                      >
                        Remove
                      </button>
                    </div>
                  ))}
                  <button
                    className="button ghost"
                    type="button"
                    onClick={() => setGroupForm({
                      ...groupForm,
                      custom: [...groupForm.custom, { key: '', value: '' }],
                    })}
                  >
                    Add Custom Label
                  </button>
                </div>
              </div>
              <div className="full inline-row">
                <button className="button" type="submit">
                  {groupForm.groupId ? 'Update Group' : 'Create Group'}
                </button>
                {groupsStatus && <span className="status">{groupsStatus}</span>}
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}
