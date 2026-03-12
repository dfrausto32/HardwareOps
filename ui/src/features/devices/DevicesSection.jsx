export default function DevicesSection({
  allDevicesSelected,
  canDecommissionDevices,
  clearDeviceSelection,
  deviceSourceFor,
  deviceStatusFilter,
  devicesError,
  devicesLoading,
  filteredDevices,
  handleBulkDecommissionSelectedDevices,
  handleDeviceRowSelect,
  handleSelectAllDevices,
  loadDevices,
  selectedDeviceId,
  selectedDeviceIds,
  selectedDeviceSet,
  setDeviceDrawerOpen,
  setDeviceFormDirty,
  setDeviceStatusFilter,
  setSelectedDeviceId,
  someDevicesSelected,
}) {
  return (
    <>
            <section id="devices" className="card">
              <div className="section-header">
                <h2>Devices</h2>
                <div className="inline-row">
                  <select
                    value={deviceStatusFilter}
                    onChange={(e) => setDeviceStatusFilter(e.target.value)}
                  >
                    <option value="all">All</option>
                    <option value="active">Active</option>
                    <option value="degraded">Degraded</option>
                    <option value="stale">Stale</option>
                    <option value="offline">Offline</option>
                  </select>
                  <button onClick={loadDevices} className="button">Refresh</button>
                </div>
              </div>
              {devicesError && <div className="error">{devicesError}</div>}
              {selectedDeviceIds.length > 1 && (
                <div className="group-selection-toolbar">
                  <div className="group-selection-count">{selectedDeviceIds.length} selected</div>
                  <div className="inline-row">
                    {canDecommissionDevices && (
                      <button
                        className="button"
                        onClick={handleBulkDecommissionSelectedDevices}
                        disabled={selectedDeviceIds.length === 0}
                      >
                        Decommission Selected
                      </button>
                    )}
                    <button className="button ghost" onClick={clearDeviceSelection} disabled={selectedDeviceIds.length === 0}>
                      Clear
                    </button>
                  </div>
                </div>
              )}
              {devicesLoading ? (
                <div className="placeholder">Loading devices...</div>
              ) : (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>
                          <input
                            type="checkbox"
                            checked={allDevicesSelected}
                            ref={(el) => {
                              if (el) {
                                el.indeterminate = someDevicesSelected
                              }
                            }}
                            onChange={(e) => handleSelectAllDevices(e.target.checked)}
                          />
                        </th>
                        <th>Device ID</th>
                        <th>Status</th>
                        <th>Last Seen</th>
                        <th>Labels</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredDevices.map((d, index) => (
                        <tr
                          key={d.deviceId}
                          className={selectedDeviceId === d.deviceId ? 'selected' : ''}
                          onClick={() => {
                            setSelectedDeviceId(d.deviceId)
                            setDeviceDrawerOpen(true)
                            setDeviceFormDirty(false)
                          }}
                        >
                          <td>
                            <input
                              type="checkbox"
                              checked={selectedDeviceSet.has(d.deviceId)}
                              onClick={(e) => e.stopPropagation()}
                              onChange={(e) => handleDeviceRowSelect(index, e.target.checked, Boolean(e.nativeEvent?.shiftKey))}
                            />
                          </td>
                          <td>{d.deviceId}</td>
                          <td>
                            <span className={`pill status ${(d.status || 'unknown').toLowerCase()}`}>
                              {d.status || 'unknown'}
                            </span>
                            {deviceSourceFor(d) === 'manual' && (
                              <span className="pill source manual">manual</span>
                            )}
                            {deviceSourceFor(d) === 'group' && (
                              <span className="pill source group">group</span>
                            )}
                            {deviceSourceFor(d) === 'agent' && (
                              <span className="pill source agent">agent</span>
                            )}
                          </td>
                          <td>{d.lastSeen || '—'}</td>
                          <td><code>{d.labels ? JSON.stringify(d.labels) : '—'}</code></td>
                        </tr>
                      ))}
                      {filteredDevices.length === 0 && (
                        <tr>
                          <td colSpan={5}>No devices match this filter.</td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
    </>
  )
}
