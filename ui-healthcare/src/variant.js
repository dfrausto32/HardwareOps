import CompliancePage from './features/healthcare/CompliancePage'
import HipaaAuditPage from './features/healthcare/HipaaAuditPage'

export const variant = {
  brandName: 'HC Device Console',
  pageTitle: 'HC Device Console',
  // 'global' omitted — Global Plane not exposed to healthcare customers
  allowedViews: ['dashboard', 'metrics', 'logs', 'security', 'settings'],
  extraViews: [
    {
      id: 'compliance',
      label: 'Compliance',
      icon: 'icon-compliance',
      requiredRole: 'viewer',
      component: CompliancePage,
    },
    {
      id: 'hipaa-audit',
      label: 'HIPAA Audit',
      icon: 'icon-logs',
      requiredRole: 'admin',
      component: HipaaAuditPage,
    },
  ],
  themeOverrides: {
    accent: '#1a6fca',
    accentLight: '#1558a8',
  },
  featureFlags: {
    embeddedMode: true,
  },
}
