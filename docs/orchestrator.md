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

### POST /api/v1/nodes/register

Register or update a sandboxd node endpoint.

- Success: `200 OK`
- Failure: `400 Bad Request` (invalid request body or invalid `name/ip/port`)
- Failure: `500 Internal Server Error` (storage or internal error)

**Request**

```json
{
    "name": "node-a",
    "ip": "192.168.0.3",
    "port": 8080
}
```

**Response**

```json
{
    "node": {
        "name": "node-a",
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
        "sandboxd_base_url": "http://192.168.0.3:8080"
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
            "sandboxd_base_url": "http://192.168.0.3:8080"
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
        "sandboxd_base_url": "http://192.168.0.3:8080",
        "resources": {
            "available_cpu_milli": 3500,
            "available_memory_bytes": 14904602214,
            "max_alloc_percent": 90
        }
    }
}
```

### DELETE /api/v1/nodes/{name}

Delete a node registration.

- Success: `200 OK`
- Failure: `400 Bad Request` (empty/invalid name)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "deleted": "node-a"
}
```

### POST /api/v1/nodes/{name}/heartbeat

Trigger immediate health/resource probe against sandboxd.

- Success: `200 OK`
- Failure: `404 Not Found` (`node not found`)
- Failure: `500 Internal Server Error`

**Response**

```json
{
    "node": {
        "name": "node-a",
        "state": "Ready",
        "sandboxd_base_url": "http://192.168.0.3:8080"
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
        "ttlSeconds": 3600,
        "ports": [
            {
                "containerPort": 80,
                "protocol": "tcp"
            }
        ],
        "containers": [
            {
                "name": "web",
                "image": "nginx:latest",
                "args": [],
                "env": [],
                "workDir": "",
                "resource": {
                    "cpu": "200m",
                    "memory": "256Mi"
                }
            }
        ]
    }
}
```

**Response**

```json
{
    "sandbox": {
        "id": "sbx-demo-1",
        "spec": {
            "egress": true,
            "ttlSeconds": 3600,
            "ports": [
                {
                    "containerPort": 80,
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
            "expireAt": "2026-05-15T05:31:30Z"
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
                        "containerPort": 80,
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
                "phase": "Running",
                "nodeName": "node-a",
                "assignedPorts": [
                    {
                        "hostPort": 10000,
                        "containerPort": 80,
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
            "nodeName": "node-a",
            "assignedPorts": [
                {
                    "hostPort": 10000,
                    "containerPort": 80,
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

## Node Proxy APIs (Pass-through to sandboxd)

All endpoints below require `{name}` to be an existing node in orchestrator.

If node lookup fails: `404 Not Found`.

If upstream sandboxd call fails or returns invalid data: `502 Bad Gateway`.

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
    "external_ip": "203.0.113.10"
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
    "external_ip": "203.0.113.10"
}
```

### POST /api/v1/nodes/{name}/sandboxes

Create sandbox directly on selected node (sandboxd API pass-through).

- Success: `200 OK`
- Failure: `400 Bad Request` (invalid JSON body)

**Request**

```json
{
    "id": "sbx-direct-1",
    "egress": true,
    "ports": [
        {
            "hostPort": 30080,
            "containerPort": 80,
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
}
```

**Response Example**

```json
{
    "sandbox": {
        "id": "sbx-direct-1",
        "phase": "creating"
    },
    "external_ip": "203.0.113.10"
}
```

### DELETE /api/v1/nodes/{name}/sandboxes/{id}

- Success: `200 OK`

**Response Example**

```json
{
    "id": "sbx-direct-1",
    "phase": "deleted",
    "external_ip": "203.0.113.10"
}
```

### GET /api/v1/nodes/{name}/sandboxes/{id}/containers/{container}/logs

Query params:

- `cursor` (optional)
- `limit` (optional, default `100`)

- Success: `200 OK`

**Response Example**

```json
{
    "sandbox_id": "sbx-demo-1",
    "container": "web",
    "logs": {
        "lines": ["line1", "line2"],
        "next_cursor": "1234",
        "has_more": false
    },
    "external_ip": "203.0.113.10"
}
```

### POST /api/v1/nodes/{name}/reconcile

Trigger sandboxd reconcile on selected node.

- Success: `200 OK`

**Response Example**

```json
{
    "ok": true,
    "external_ip": "203.0.113.10"
}
```

## Field Semantics

### Node `state`

- `Unknown`: node exists but readiness not yet converged
- `Ready`: heartbeat success streak reached threshold
- `NotReady`: heartbeat failure streak reached threshold

### Sandbox `status.phase`

- `Pending`: object created, not yet scheduled
- `Scheduled`: node and host ports assigned, runtime creation in progress
- `Running`: sandbox created on node
- `Failed`: scheduling/runtime operation failed
- `Deleting`: delete flow in progress

### Sandbox `status.assignedPorts`

Resolved host port mapping used by scheduler and runtime provisioning:

- `hostPort`: selected host-side port (dynamic if `0` requested)
- `containerPort`: target container port
- `protocol`: `tcp` or `udp`

## Environment Variables

- `ORCH_CREATE_RPS`  
  Create-sandbox API token refill rate (requests per second).  
  Default: `20`

- `ORCH_CREATE_BURST`  
  Create-sandbox API token bucket size (burst capacity).  
  Default: `40`
