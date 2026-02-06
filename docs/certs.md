# TLS + CA Setup (On‑Prem)

This uses a local internal CA to issue the server certificate for `hardwareops.internal`.

## 1) Generate CA
```
sudo OUT_DIR=/opt/hardwareops/certs ./scripts/bootstrap-ca.sh
```

Outputs:
- `/opt/hardwareops/certs/ca.crt`
- `/opt/hardwareops/certs/ca.key`

## 2) Issue server cert
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/issue-server-cert.sh
```

### Fast path (CA + server cert + env)
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```

If you change domains or see TLS errors (AKI/SKI mismatch), reissue with:
```
sudo FORCE=1 OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```

Outputs:
- `/opt/hardwareops/certs/server.crt`
- `/opt/hardwareops/certs/server.key`

## 3) Trust the CA on clients
### Linux (system trust)
```
sudo cp /opt/hardwareops/certs/ca.crt /usr/local/share/ca-certificates/hardwareops-ca.crt
sudo update-ca-certificates
```

### Windows
1. Open `ca.crt`
2. Install to **Trusted Root Certification Authorities**

### macOS
1. Add `ca.crt` to Keychain Access
2. Set to **Always Trust**

## 4) Agent trust (no system trust required)
Agents can point directly to the CA:
```
CONTROL_PLANE_CA_CERT_PATH=/etc/hardwareops/agent/certs/ca.crt
```
