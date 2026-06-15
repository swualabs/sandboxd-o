---
title: Orchestrator API
nav_order: 2
---

Base URL: `http://localhost:8082`

## Common Error Response

```json
{
    "error": "error message"
}
```

## Health API

### GET /healthz

- Success: `200 OK`

**Response**

```json
{
    "ok": true
}
```

## Node APIs

### POST /api/v1/nodes

Create or update a node object.

- Success: `200 OK`
- Failure: `400 Bad Request` (invalid request body or invalid `id/ip/port`)
- Failure: `500 Internal Server Error` (storage or internal error)

**Request**

```json
{
    "id": "node-a",
    "spec": {
        "ip": "192.168.0.3",
        "port": 8080
    }
}
```

**Response**

```json
{
    "node": {
        "id": "node-a",
        "ip": "192.168.0.3",
        "port": 8080,
        "state": "Unknown",
        "source": "api",
        "success_streak": 0,
        "failure_streak": 0,
        "created_at": "2026-05-15T04:30:00Z",
        "updated_at": "2026-05-15T04:30:00Z",
        "resources": {
            "capacity_cpu_milli": 4000,
            "capacity_memory_bytes": 16709799936,
            "allocatable_cpu_milli": 3600,
            "allocatable_memory_bytes": 15038819942,
            "used_cpu_milli": 0,
            "used_memory_bytes": 0,
            "available_cpu_milli": 3600,
            "available_memory_bytes": 15038819942,
            "max_alloc_percent": 90,
            "updated_at": "2026-05-15T04:30:00Z"
        },
        "sbxlet_base_url": "http://192.168.0.3:8080"
    }
}
```

### GET /api/v1/nodes

List all registered nodes.

- Success: `200 OK`
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "items": [
        {
            "name": "node-a",
            "ip": "192.168.0.3",
            "port": 8080,
            "state": "Ready",
            "source": "api",
            "last_error": "",
            "success_streak": 2,
            "failure_streak": 0,
            "created_at": "2026-05-15T04:30:00Z",
            "updated_at": "2026-05-15T04:31:30Z",
            "last_heartbeat": "2026-05-15T04:31:30Z",
            "resources": {
                "capacity_cpu_milli": 4000,
                "capacity_memory_bytes": 16709799936,
                "allocatable_cpu_milli": 3600,
                "allocatable_memory_bytes": 15038819942,
                "used_cpu_milli": 100,
                "used_memory_bytes": 134217728,
                "available_cpu_milli": 3500,
                "available_memory_bytes": 14904602214,
                "max_alloc_percent": 90,
                "updated_at": "2026-05-15T04:31:30Z"
            },
            "sbxlet_base_url": "http://192.168.0.3:8080"
        }
    ]
}
```

### GET /api/v1/nodes/{name}

Get a single node.

- Success: `200 OK`
- Failure: `404 Not Found` (`node not found`)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "node": {
        "name": "node-a",
        "ip": "192.168.0.3",
        "port": 8080,
        "state": "Ready",
        "source": "api",
        "success_streak": 2,
        "failure_streak": 0,
        "sbxlet_base_url": "http://192.168.0.3:8080",
        "resources": {
            "available_cpu_milli": 3500,
            "available_memory_bytes": 14904602214,
            "max_alloc_percent": 90,
            "external": "203.0.113.10"
        }
    }
}
```

### DELETE /api/v1/nodes/{name}

Delete a node registration.

- Query: `force` (optional, boolean)
    - `false` or omitted: orchestrator calls node APIs (sandbox delete/reconcile) and fails node deletion if those calls fail.
    - `true`: orchestrator skips node API failures and force-deletes node metadata.
    - Use `force=true` only when the node is already confirmed gone/unreachable.
- Success: `200 OK`
- Failure: `400 Bad Request` (empty/invalid name)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "deleted": "node-a"
}
```

Examples:

- Normal delete: `DELETE /api/v1/nodes/node-a`
- Force delete: `DELETE /api/v1/nodes/node-a?force=true`

### POST /api/v1/nodes/{name}/heartbeat

Trigger immediate health/resource probe against sbxlet.

- Success: `200 OK`
- Failure: `404 Not Found` (`node not found`)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "node": {
        "name": "node-a",
        "state": "Ready",
        "sbxlet_base_url": "http://192.168.0.3:8080"
    },
    "heartbeat": "ok",
    "resources": {
        "capacity_cpu_milli": 4000,
        "capacity_memory_bytes": 16709799936,
        "allocatable_cpu_milli": 3600,
        "allocatable_memory_bytes": 15038819942,
        "used_cpu_milli": 100,
        "used_memory_bytes": 134217728,
        "available_cpu_milli": 3500,
        "available_memory_bytes": 14904602214,
        "max_alloc_percent": 90,
        "updated_at": "2026-05-15T04:31:30Z"
    },
    "heartbeat_error": "",
    "status_error": ""
}
```

`heartbeat` values:

- `ok`
- `failed`

## Control-Plane Sandbox APIs

### POST /api/v1/sandboxes

Create a control-plane sandbox object for scheduling and reconciliation.

- Success: `201 Created`
- Failure: `400 Bad Request` (validation)
- Failure: `500 Internal Server Error`

**Request**

```json
{
    "id": "sbx-demo-1",
    "spec": {
        "egress": true,
        "ttl_seconds": 3600,
        "ports": [
            {
                "container_port": 80,
                "protocol": "tcp"
            }
        ],
        "volumes": [
            {
                "name": "runtime-state",
                "ephemeral_storage": "128Mi"
            }
        ],
        "readiness_probe": {
            "protocol": "http",
            "port": 80,
            "path": "/",
            "initial_delay_seconds": 1,
            "period_seconds": 1,
            "timeout_seconds": 1,
            "success_threshold": 2,
            "failure_threshold": 3
        },
        "containers": [
            {
                "name": "web",
                "image": "nginx:latest",
                "args": [],
                "env": [],
                "work_dir": "",
                "volume_mounts": [
                    {
                        "name": "runtime-state",
                        "mount_path": "/usr/share/nginx/html"
                    }
                ],
                "resource": {
                    "cpu": "200m",
                    "memory": "256Mi",
                    "ephemeral_storage": "96Mi"
                }
            }
        ]
    }
}
```

- `spec.readiness_probe` is optional.
- `spec.volumes` is optional and defines sandbox-local shared tmpfs volumes.
- `spec.containers[].volume_mounts` is optional and may reference entries declared in `spec.volumes`.
- Shared volumes are never shared across sandboxes and are deleted with the sandbox.
- `spec.containers[].volume_mounts[].mount_path` must be an absolute path and cannot be `/tmp`.
- If set, all readiness probe fields are required.
- `protocol` supports `tcp` and `http`. If `http` is used, `path` is required and must start with `/`.

**Response**

```json
{
    "sandbox": {
        "id": "sbx-demo-1",
        "spec": {
            "egress": true,
            "ttl_seconds": 3600,
            "ports": [
                {
                    "container_port": 80,
                    "protocol": "tcp"
                }
            ],
            "containers": [
                {
                    "name": "web",
                    "image": "nginx:latest",
                    "resource": {
                        "cpu": "200m",
                        "memory": "256Mi"
                    }
                }
            ]
        },
        "status": {
            "phase": "Pending",
            "expire_at": "2026-05-15T05:31:30Z"
        },
        "created_at": "2026-05-15T04:31:30Z",
        "updated_at": "2026-05-15T04:31:30Z"
    }
}
```

### GET /api/v1/sandboxes

List all control-plane sandbox objects.

- Success: `200 OK`
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "items": [
        {
            "id": "sbx-demo-1",
            "spec": {
                "egress": true,
                "ports": [
                    {
                        "container_port": 80,
                        "protocol": "tcp"
                    }
                ],
                "volumes": [
                    {
                        "name": "runtime-state",
                        "ephemeral_storage": "128Mi"
                    }
                ],
                "containers": [
                    {
                        "name": "web",
                        "image": "nginx:latest",
                        "volume_mounts": [
                            {
                                "name": "runtime-state",
                                "mount_path": "/usr/share/nginx/html"
                            }
                        ],
                        "resource": {
                            "cpu": "200m",
                            "memory": "256Mi",
                            "ephemeral_storage": "96Mi"
                        }
                    }
                ]
            },
            "status": {
                "phase": "Running",
                "node_name": "node-a",
                "external": "203.0.113.10",
                "assigned_ports": [
                    {
                        "host_port": 10000,
                        "container_port": 80,
                        "protocol": "tcp"
                    }
                ]
            },
            "created_at": "2026-05-15T04:31:30Z",
            "updated_at": "2026-05-15T04:31:34Z"
        }
    ]
}
```

### GET /api/v1/sandboxes/{id}

Get one control-plane sandbox object.

- Success: `200 OK`
- Failure: `404 Not Found` (`sandbox not found`)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "sandbox": {
        "id": "sbx-demo-1",
        "status": {
            "phase": "Running",
            "node_name": "node-a",
            "external": "203.0.113.10",
            "assigned_ports": [
                {
                    "host_port": 10000,
                    "container_port": 80,
                    "protocol": "tcp"
                }
            ]
        },
        "created_at": "2026-05-15T04:31:30Z",
        "updated_at": "2026-05-15T04:31:34Z"
    }
}
```

### DELETE /api/v1/sandboxes/{id}

Delete one control-plane sandbox object.

- Success: `200 OK`
- Failure: `400 Bad Request` (invalid id)
- Failure: `404 Not Found` (`sandbox not found`)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "deleted": "sbx-demo-1"
}
```

## Node Proxy APIs (Pass-through to sbxlet)

All endpoints below require `{name}` to be an existing node in orchestrator.

If node lookup fails: `404 Not Found`.

If upstream sbxlet call fails or returns invalid data: `502 Bad Gateway`.

### GET /api/v1/nodes/{name}/sandboxes

Query params:

- `cursor` (optional)
- `limit` (optional, default `20`)

- Success: `200 OK`

**Response Example**

```json
{
    "items": [
        {
            "id": "sbx-demo-1",
            "phase": "running"
        }
    ],
    "next_cursor": "sbx-demo-1",
    "external": "203.0.113.10"
}
```

### GET /api/v1/nodes/{name}/sandboxes/{id}

- Success: `200 OK`

**Response Example**

```json
{
    "sandbox": {
        "id": "sbx-demo-1",
        "phase": "running"
    },
    "external": "203.0.113.10"
}
```

### POST /api/v1/nodes/{name}/sandboxes

Create sandbox directly on selected node (sbxlet API pass-through).

- Success: `200 OK`
- Failure: `400 Bad Request` (invalid JSON body)

**Request**

```json
{
    "id": "sbx-direct-1",
    "egress": true,
    "ports": [
        {
            "host_port": 30080,
            "container_port": 80,
            "protocol": "tcp"
        }
    ],
    "volumes": [
        {
            "name": "runtime-state",
            "ephemeral_storage": "128Mi"
        }
    ],
    "containers": [
        {
            "name": "web",
            "image": "nginx:latest",
            "volume_mounts": [
                {
                    "name": "runtime-state",
                    "mount_path": "/usr/share/nginx/html"
                }
            ],
            "resource": {
                "cpu": "200m",
                "memory": "256Mi",
                "ephemeral_storage": "96Mi"
            }
        }
    ]
}
```

**Response Example**

```json
{
    "sandbox": {
        "id": "sbx-direct-1",
        "phase": "creating"
    }
}
```

### DELETE /api/v1/nodes/{name}/sandboxes/{id}

- Success: `200 OK`

**Response Example**

```json
{
    "id": "sbx-direct-1",
    "phase": "deleted"
}
```

### GET /api/v1/nodes/{name}/sandboxes/{id}/logs

- Success: `200 OK`
- CRI-formatted log lines are returned sorted by timestamp across containers.

**Response Example**

```json
{
    "sandbox_id": "sbx-demo-1",
    "logs": {
        "lines": ["[app] line1", "[db] line2"]
    }
}
```

The older `/api/v1/nodes/{name}/sandboxes/{id}/containers/{container}/logs` proxy remains available for per-container compatibility.

### POST /api/v1/nodes/{id}/reconcile

Trigger sbxlet reconcile on selected node.

- Success: `200 OK`

**Response Example**

```json
{
    "ok": true
}
```

## Field Semantics

### Node `state`

- `Unknown`: node exists but readiness not yet converged
- `Ready`: heartbeat success streak reached threshold
- `NotReady`: heartbeat failure streak reached threshold

### Sandbox `status.phase`

- `Pending`: object created, not yet scheduled
- `Scheduled`: node and host ports assigned; sbxlet runtime creation/readiness in progress
- `Running`: sbxlet reports sandbox `running` (including readiness probe success when configured)
- `Failed`: scheduling/runtime operation failed
- `Deleting`: delete flow in progress

### Sandbox `status.assigned_ports`

Resolved host port mapping used by scheduler and runtime provisioning:

- `host_port`: orchestrator-assigned host-side port (automatically selected within configured range)
    - In `spec.ports`, client-sent `host_port` is accepted for backward compatibility but ignored.
- `container_port`: target container port
- `protocol`: `tcp` or `udp`

### Node `resources.external` and Sandbox `status.external`

- `resources.external`: value from `External` object bound to node (`POST /api/v1/externals`).
- `status.external`: snapshot copied from node resources during scheduling/status sync.
- If no external is configured, value is `(none)`.
- API responses use `external` field.

### Sandbox Private IP Sync

- Sandbox private IP is synchronized from sbxlet sandbox status responses.
- Orchestrator updates `sandbox.status.ip` during status sync.
- Orchestrator also performs a short best-effort fetch right after create scheduling, so `ip` can appear before `Running`.

### Readiness Probe Semantics

- `spec.readiness_probe` is forwarded to sbxlet as runtime readiness configuration.
- If `readiness_probe` is omitted, sbxlet transitions to `running` immediately after provisioning is complete.
- If `readiness_probe` is set, sbxlet stays `creating` until probe success threshold is met; failures beyond threshold transition sbxlet to `error`, which syncs to orchestrator `Failed`.
- Readiness is evaluated against sandbox-private endpoints (`sandboxIP:port`). This does not guarantee external/published host-port reachability from every client network path.

## Configuration

- Default config file: `/var/lib/sandboxd/sbxorch_config.json`
- Example config: `configs/sbxorch_config.json`
- Compatible environment variable overrides are still supported for backward compatibility, including `ORCH_NODE_PROBE_TIMEOUT`, `ORCH_SANDBOX_OP_TIMEOUT`, `ORCH_CREATE_RPS`, and `ORCH_CREATE_BURST`, but JSON config is the recommended mechanism.
