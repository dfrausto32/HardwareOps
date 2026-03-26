# Global Desired State (Policy Push)

E3 adds a **global desired state** layer to HardwareOps that lets operators define label-based device groups and push deployment policies cross-region from a single global plane.

## Concepts

| Concept | Description |
|---|---|
| **Global group** | A named label-selector rule (`{"env":"prod","tier":"edge"}`). Devices in any regional plane whose labels are a superset of the selector match the group. |
| **Global desired state** | An artifact ID + version + config rev + policy JSON + optional checkin interval assigned to a global group. |
| **Policy push** | When a desired state is set, the global plane fans out to all enabled regional planes via `POST /api/v1/federation/policies`. |
| **Policy cache** | Each regional control plane stores received policies in `global_policy_cache`. The cache is consulted during device checkin as the lowest-priority fallback. |

## Resolution order during device checkin

1. **Device-manual** — explicit desired state set on the device record directly.
2. **Local group** — regional control-plane group whose selector matches the device.
3. **Global policy cache** — cross-region policy received from the global plane.
4. **Device agent default** — agent-side no-op / keep-current-version logic.

Higher priority entries win; global policy only applies when no local override exists.

## API — Global plane

### Groups

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/groups` | viewer | List all global groups |
| `POST` | `/api/v1/groups` | operator | Create a global group |
| `DELETE` | `/api/v1/groups/{groupId}` | admin | Delete group and fan-out delete to regional planes |

**Create group** request body:
```json
{
  "name": "fleet-prod",
  "selectorJson": {"env": "prod", "region": "us-east-1"}
}
```

### Desired state

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/desired-state` | viewer | List all desired states (joined with group metadata) |
| `GET` | `/api/v1/groups/{groupId}/desired-state` | viewer | Get desired state for a specific group |
| `PUT` | `/api/v1/groups/{groupId}/desired-state` | operator | Set desired state + fan-out push to regional planes |
| `DELETE` | `/api/v1/groups/{groupId}/desired-state` | operator | Remove desired state + fan-out delete |

**Set desired state** request body:
```json
{
  "artifactId": "550e8400-e29b-41d4-a716-446655440000",
  "desiredVersion": "1.2.3",
  "desiredConfigRev": "abc123",
  "checkinInterval": 300
}
```

All fields are optional — omit to clear a field.

## API — Regional plane (federation receiver)

These endpoints are called by the global plane, not by operators directly. They require a service token with the `federation.push` scope.

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/federation/policies` | Upsert a received global policy |
| `GET` | `/api/v1/federation/policies` | List cached policies (debug / audit) |
| `DELETE` | `/api/v1/federation/policies/{groupId}` | Remove a cached policy |

## Database

### Global plane tables

**`global_groups`**
```sql
group_id UUID PRIMARY KEY,
name TEXT,
selector_json JSONB,
created_by TEXT,
created_at TIMESTAMPTZ,
updated_at TIMESTAMPTZ
```

**`global_desired_state`**
```sql
group_id UUID REFERENCES global_groups(group_id) ON DELETE CASCADE,
artifact_id TEXT,
desired_version TEXT,
desired_config_rev TEXT,
policy_json JSONB,
components_json JSONB,
checkin_interval INT,
updated_at TIMESTAMPTZ,
updated_by TEXT
```

### Regional plane table

**`global_policy_cache`**
```sql
cache_id UUID PRIMARY KEY,
group_id UUID UNIQUE,
group_name TEXT,
selector_json JSONB,
artifact_id TEXT,
desired_version TEXT,
desired_config_rev TEXT,
policy_json JSONB,
components_json JSONB,
checkin_interval INT,
received_at TIMESTAMPTZ,
updated_at TIMESTAMPTZ
```

## Configuration

No additional environment variables are required for E3. The global plane uses the same `TOKEN_ENCRYPTION_KEY` for encrypting regional plane service tokens stored in `regional_planes`.

## UI

The **Global Plane → Groups** tab in the web console provides:

- List of all global groups with their current desired states.
- **Create group** modal — name + label selector JSON.
- **Set policy** modal — artifact ID, desired version, config rev, checkin interval.

Policies are immediately pushed to all enabled regional planes upon save.

## Failure handling

Fan-out is **fire-and-forget** from the HTTP response perspective — the API returns success as soon as the state is persisted in the global plane store. Each regional push runs in a separate goroutine with a 15-second timeout. Push failures are logged server-side. Regions that miss a push will receive the policy on the next sync cycle (E4 — Sync reconciler).

## Migration

Run migrations on both the global plane and every regional plane:

```bash
# Global plane
AUTO_MIGRATE=1 ./global-plane

# Regional plane
AUTO_MIGRATE=1 ./control-plane
```

Migration files:
- Global: `migrations/global/0007_global_groups_desired_state.sql`
- Regional: `migrations/0034_global_policy_cache.sql`
