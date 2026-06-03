export default function CompliancePage({ view }) {
  if (view !== 'compliance') return null
  return (
    <section id="compliance" className="card settings-card">
      <div className="section-header settings-header">
        <h2>Compliance</h2>
      </div>
      <div className="muted" style={{ padding: '40px 0', textAlign: 'center' }}>
        Compliance dashboard — coming soon.
      </div>
    </section>
  )
}
