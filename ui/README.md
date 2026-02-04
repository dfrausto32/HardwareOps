# UI

React + Vite console scaffold.

## Prereqs
- Node.js 18+

## Setup
```
cd ui
npm install
npm run dev
```

## Env
- `VITE_API_BASE_URL` (default `https://localhost:8080`)
- `VITE_WS_BASE_URL` (optional override for WebSocket base, defaults derived from API base)
- `VITE_SIMULATE_PROD=1` to disable the dev proxy and force real TLS behavior (recommended)
- `VITE_API_PROXY=1` to proxy `/api` and `/healthz` through the Vite dev server
- `VITE_TLS=1` to default the API base to `https://localhost:8080` when not set

### Self-signed TLS (dev)
Browsers won't allow insecure TLS from JS. Use one of these:
- Trust the dev CA in your OS and use direct HTTPS.
- Or enable the dev proxy (recommended for local dev):

```
VITE_API_PROXY=1 VITE_API_BASE_URL=https://localhost:8080 npm run dev
```
The proxy uses `secure: false` to allow the self-signed cert.

## Live events + logs
- The UI connects to `ws(s)://<base>/api/v1/events` for live device check-in/apply updates.
- Logs are in the dedicated **Logs** page in the top nav (download CSV or view inline).

If the WebSocket proxy errors with `EPIPE`, restart Vite after changing envs and ensure the control-plane
is running with `DISABLE_HTTP2=1` (WebSockets require HTTP/1.1).

## Realistic dev mode (recommended)
To simulate production behavior:
1) Trust the dev CA in your OS (or use Caddy with a trusted cert).
2) Set `VITE_SIMULATE_PROD=1` and do **not** enable the proxy.
3) Ensure the control-plane allows the UI origin:
```
export CORS_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
```
3) Use direct HTTPS/WSS:
```
VITE_API_BASE_URL=https://localhost:8080
VITE_SIMULATE_PROD=1
npm run dev
```

## UI structure
- Left nav with two pages: **Dashboard** and **Logs**.
- Devices live on the Dashboard; clicking a device opens a side drawer with details + desired state editor.
- Dark mode is default with a toggle to light mode.
- Logs page supports time range filtering and sort order (newest/oldest) with color-coded rows by level.
