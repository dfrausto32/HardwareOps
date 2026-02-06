# CoreDNS (On‑Prem Local DNS)

This sets up a simple local DNS server so all clients can resolve `hardwareops.internal`.
Avoid `.local` domains because most Linux distros treat them as **mDNS** and browsers/curl won’t query unicast DNS.
For WSL + VirtualBox testing, **run CoreDNS on the control‑plane VM**, not inside WSL.

## 1) Choose a DNS host
Run CoreDNS on the same server as the control‑plane or a dedicated DNS box.

## 2) Configure the zone
Fastest path is to use the helper script:
```
DNS_IP=<CONTROL_PLANE_IP> ./scripts/run-coredns.sh
```
This generates a zone file with:
- `@ IN A <CONTROL_PLANE_IP>`
- `ns1 IN A <CONTROL_PLANE_IP>`

Manual path (edit the sample zone file):
```
deploy/dns/db.hardwareops.internal
```
Update these lines:
```
ns1 IN A <CONTROL_PLANE_IP>
@   IN A <CONTROL_PLANE_IP>
```

## 3) Run CoreDNS (Docker)
```
docker run -d --name coredns \
  -p 53:53/udp -p 53:53/tcp \
  -v $(pwd)/deploy/dns/Corefile:/etc/coredns/Corefile:ro \
  -v $(pwd)/deploy/dns/db.hardwareops.internal:/etc/coredns/db.hardwareops.internal:ro \
  coredns/coredns:1.11.1 -conf /etc/coredns/Corefile
```

If the control‑plane VM itself cannot query `127.0.0.1:53`, run CoreDNS on the host network:
```
DNS_IP=<CONTROL_PLANE_IP> HOST_NET=1 ./scripts/run-coredns.sh
```

### Port 53 already in use (Ubuntu/systemd‑resolved)
If you see `failed to bind host port 0.0.0.0:53`, free the port:
```
sudo ss -lntup | grep ':53 '
sudo systemctl stop systemd-resolved
sudo systemctl disable systemd-resolved
sudo rm -f /etc/resolv.conf
echo "nameserver 1.1.1.1" | sudo tee /etc/resolv.conf
```
Then run CoreDNS again. After CoreDNS is up, you can point DNS back to the server:
```
DNS_SERVER=<CONTROL_PLANE_IP> ./scripts/set-dns.sh
```

## 4) Point clients to DNS
Set your client machines to use the CoreDNS server IP as their DNS server.

Linux (systemd‑resolved or fallback):
```
DNS_SERVER=<DNS_SERVER_IP> ./scripts/set-dns.sh
```
If `resolvectl` is not available, force manual mode:
```
DNS_SERVER=<DNS_SERVER_IP> MODE=manual ./scripts/set-dns.sh
```

Windows:
1. Network adapter → Properties → IPv4 → Use the following DNS server
2. Set the CoreDNS IP as primary.

macOS:
1. System Settings → Network → DNS
2. Add the CoreDNS IP.

## 5) Verify
```
nslookup hardwareops.internal <DNS_SERVER_IP>
```

You should see the control‑plane server IP returned.

## WSL + VirtualBox note
Running CoreDNS inside WSL is **not recommended** because the WSL IP is NATed and not stable.
Use the control‑plane VM (bridged networking) as the DNS host, and point agent VMs to it.

## Control‑plane VM note
If CoreDNS is running **on the control‑plane VM itself**, you can either:

**A) Use /etc/hosts for local checks (simplest)**  
```
echo "127.0.0.1 hardwareops.internal" | sudo tee -a /etc/hosts
```

**B) Point the CP VM resolver to localhost (recommended for full DNS flow)**  
```
DNS_SERVER=127.0.0.1 MODE=manual ./scripts/set-dns.sh
```

Verify from the agent VM (not the CP VM):
```
nslookup hardwareops.internal <CONTROL_PLANE_IP>
```
