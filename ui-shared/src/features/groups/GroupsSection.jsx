export default function GroupsSection({
  allGroupsSelected,
  canDecommissionDevices,
  canEditDeviceLabels,
  canManageDesiredState,
  canManageGroups,
  clearGroupSelection,
  formatSelector,
  groupBatchError,
  groupBatchStatus,
  groupCounts,
  groupDeviceList,
  groups,
  groupsError,
  handleBulkDeleteSelectedGroups,
  handleDeleteGroup,
  handleGroupDeviceToggle,
  handleGroupRowSelect,
  handleSelectAllGroups,
  loadGroups,
  normalizeObject,
  openGroupMultiDesired,
  openGroupMultiEdit,
  selectedGroup,
  selectedGroupId,
  selectedGroupIds,
  selectedGroupSelector,
  selectedGroupSet,
  selectorToForm,
  setGroupBulkError,
  setGroupBulkOpen,
  setGroupBulkStatus,
  setGroupDesiredOpen,
  setGroupForm,
  setGroupModalOpen,
  setSelectedGroupId,
  someGroupsSelected,
}) {
  return (
    <>
            <section id="groups" className="card">
              <div className="section-header">
                <h2>Groups</h2>
                <div className="inline-row">
                  {canManageGroups && (
                    <button
                      onClick={() => {
                        setGroupForm({ groupId: '', name: '', region: '', role: '', site: '', custom: [] })
                        setGroupModalOpen(true)
                      }}
                      className="button"
                    >
                      Add Group
                    </button>
                  )}
                  {canManageGroups && (
                    <button
                      onClick={() => {
                        setGroupBulkOpen(true)
                        setGroupBulkError('')
                        setGroupBulkStatus('')
                      }}
                      className="button ghost"
                    >
                      Advanced CSV
                    </button>
                  )}
                  <button onClick={loadGroups} className="button ghost">Refresh</button>
                </div>
              </div>
              {groupsError && <div className="error">{groupsError}</div>}
              {groupBatchError && <div className="error">{groupBatchError}</div>}

              {selectedGroupIds.length > 1 && (
                <div className="group-selection-toolbar">
                  <div className="group-selection-count">{selectedGroupIds.length} selected</div>
                  <div className="inline-row">
                    {canManageGroups && (
                      <button className="button ghost" onClick={openGroupMultiEdit} disabled={selectedGroupIds.length === 0}>
                        Edit Selected
                      </button>
                    )}
                    {canManageDesiredState && (
                      <button className="button ghost" onClick={openGroupMultiDesired} disabled={selectedGroupIds.length === 0}>
                        Set Desired Selected
                      </button>
                    )}
                    {canManageGroups && (
                      <button className="button danger" onClick={() => handleBulkDeleteSelectedGroups(false)} disabled={selectedGroupIds.length === 0}>
                        Delete Selected
                      </button>
                    )}
                    {canManageGroups && canDecommissionDevices && (
                      <button className="button ghost" onClick={() => handleBulkDeleteSelectedGroups(true)} disabled={selectedGroupIds.length === 0}>
                        Delete + Devices
                      </button>
                    )}
                    <button className="button ghost" onClick={clearGroupSelection} disabled={selectedGroupIds.length === 0}>
                      Clear
                    </button>
                  </div>
                  {groupBatchStatus && <div className="status">{groupBatchStatus}</div>}
                </div>
              )}

              {groups.length === 0 ? (
                <div className="empty-state">
                  <div className="empty-state-title">No groups yet</div>
                  <div className="empty-state-hint">Groups let you apply desired state to multiple devices at once.</div>
                </div>
              ) : (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>
                        <input
                          type="checkbox"
                          checked={allGroupsSelected}
                          ref={(el) => {
                            if (el) {
                              el.indeterminate = someGroupsSelected
                            }
                          }}
                          onChange={(e) => handleSelectAllGroups(e.target.checked)}
                        />
                      </th>
                      <th>Name</th>
                      <th>Selector</th>
                      <th>Devices</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {groups.map((group, index) => (
                      <tr key={group.groupId} className={selectedGroupId === group.groupId ? 'selected' : ''}>
                        <td>
                          <input
                            type="checkbox"
                            checked={selectedGroupSet.has(group.groupId)}
                            onChange={(e) => handleGroupRowSelect(index, e.target.checked, Boolean(e.nativeEvent?.shiftKey))}
                          />
                        </td>
                        <td>{group.name || group.groupId}</td>
                        <td><code>{formatSelector(group.selector || {})}</code></td>
                        <td>{groupCounts[group.groupId] ?? 0}</td>
                        <td>
                          {canManageGroups && (
                            <button
                              className="button ghost"
                              onClick={() => {
                                const parsed = selectorToForm(normalizeObject(group.selector))
                                setGroupForm({
                                  groupId: group.groupId,
                                  name: group.name || '',
                                  region: parsed.region,
                                  role: parsed.role,
                                  site: parsed.site,
                                  custom: parsed.custom,
                                })
                                setGroupModalOpen(true)
                              }}
                            >
                              Edit
                            </button>
                          )}
                          <button
                            className="button ghost"
                            onClick={() => setSelectedGroupId(group.groupId)}
                          >
                            {canEditDeviceLabels ? 'Manage Devices' : 'View Devices'}
                          </button>
                          {canManageDesiredState && (
                            <button
                              className="button ghost"
                              onClick={() => {
                                setSelectedGroupId(group.groupId)
                                setGroupDesiredOpen(true)
                              }}
                            >
                              Set Desired
                            </button>
                          )}
                          {canManageGroups && (
                            <button
                              className="button danger"
                              onClick={() => handleDeleteGroup(group.groupId, false)}
                            >
                              Delete
                            </button>
                          )}
                          {canManageGroups && canDecommissionDevices && (
                            <button
                              className="button danger"
                              onClick={() => handleDeleteGroup(group.groupId, true)}
                            >
                              Delete + Devices
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              )}

              {selectedGroup && (
                <div className="group-devices">
                  <div className="section-header">
                    <h3>Group Devices · {selectedGroup.name || selectedGroup.groupId}</h3>
                    <button className="button ghost" onClick={() => setSelectedGroupId('')}>
                      Close
                    </button>
                  </div>
                  <div className="table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>Device</th>
                          <th>Status</th>
                          <th>Labels</th>
                          <th>Membership</th>
                          <th>Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {groupDeviceList.map((item) => (
                          <tr key={item.device.deviceId}>
                            <td>{item.device.deviceId}</td>
                            <td>{item.device.status}</td>
                            <td><code>{JSON.stringify(item.device.labels || {})}</code></td>
                            <td>{item.inGroup ? 'in group' : '—'}</td>
                            <td>
                              {canEditDeviceLabels ? (
                                <button
                                  className="button ghost"
                                  onClick={() => handleGroupDeviceToggle(selectedGroup, item.device, !item.inGroup)}
                                  disabled={Object.keys(selectedGroupSelector).length === 0}
                                >
                                  {item.inGroup ? 'Remove' : 'Add'}
                                </button>
                              ) : '—'}
                            </td>
                          </tr>
                        ))}
                        {groupDeviceList.length === 0 && (
                          <tr>
                            <td colSpan={5}>No devices available.</td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                  {Object.keys(selectedGroupSelector).length === 0 && (
                    <div className="hint">
                      {canEditDeviceLabels
                        ? 'Empty selector matches all devices. Add keys to enable membership control.'
                        : 'Empty selector matches all devices.'}
                    </div>
                  )}
                </div>
              )}
            </section>
    </>
  )
}
