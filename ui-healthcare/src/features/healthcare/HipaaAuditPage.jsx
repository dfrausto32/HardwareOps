export default function HipaaAuditPage({ view }) {
  if (view !== 'hipaa-audit') return null
  return (
    <section id="hipaa-audit" className="card settings-card">
      <div className="section-header settings-header">
        <h2>HIPAA Audit</h2>
      </div>
      <div className="muted" style={{ padding: '40px 0', textAlign: 'center' }}>
        HIPAA audit log view — coming soon.
      </div>
    </section>
  )
}
