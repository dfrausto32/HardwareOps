// Shows agent-reported artifact download progress for a device (roadmap T3).
// A download is "downloading" while bytes are flowing and "waiting" when the
// agent has deliberately paused it (stalled link, byte budget, outside the
// transfer window). Neither is an error: the agent resumes automatically.

function bytes(value) {
  const n = Number(value)
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n
  let i = -1
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`
}

function ago(value) {
  if (!value) return '—'
  const t = new Date(value).getTime()
  if (Number.isNaN(t)) return '—'
  const s = Math.max(0, Math.round((Date.now() - t) / 1000))
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.round(s / 60)}m ago`
  if (s < 86400) return `${Math.round(s / 3600)}h ago`
  return `${Math.round(s / 86400)}d ago`
}

export default function DownloadProgress({ components }) {
  const rows = Object.entries(components || {})
    .filter(([, comp]) => comp && comp.download)
    .map(([key, comp]) => [key, comp.download])
  if (rows.length === 0) return null

  return (
    <div className="component-table">
      <h4>Downloads</h4>
      <table>
        <thead>
          <tr>
            <th>Component</th>
            <th>State</th>
            <th>Progress</th>
            <th>Last progress</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(([key, d]) => {
            const total = Number(d.bytesTotal) || 0
            const done = Math.min(Number(d.bytesDone) || 0, total || Number.MAX_SAFE_INTEGER)
            const pct = total > 0 ? Math.floor((done / total) * 100) : null
            return (
              <tr key={key}>
                <td>
                  {key}
                  <div className="detail-note mono">{d.artifactId}</div>
                </td>
                <td>
                  <span className={`pill ${d.state === 'downloading' ? 'success' : ''}`}>{d.state}</span>
                  {d.reason && <div className="detail-note">{d.reason}</div>}
                </td>
                <td>
                  {pct !== null ? (
                    <progress
                      value={done}
                      max={total}
                      aria-label={`${key} download progress`}
                      style={{ width: '100%' }}
                    />
                  ) : null}
                  <div className="detail-note">
                    {bytes(d.bytesDone)}
                    {total > 0 ? ` of ${bytes(total)} (${pct}%)` : ''}
                    {d.attempts > 1 ? ` · attempt ${d.attempts}` : ''}
                  </div>
                </td>
                <td>{ago(d.lastProgressAt)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
