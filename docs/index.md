---
title: Home
nav_order: 1
---

Base URL: `http://localhost:8080`

## Health

- `GET /healthz`
- Success: `200 OK`

**Response**

```json
{
    "ok": true
}
```

## Sandbox APIs

### Create Sandbox

- `POST /v1/sandboxes`
- Body: `CreateSandboxRequest`

```json
{
    "id": "sbx-http-demo",
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
            "image": "nginx:alpine",
            "args": [],
            "env": [],
            "workDir": "",
            "limits": {
                "memoryBytes": 134217728,
                "cpuQuota": 50000,
                "cpuPeriod": 100000,
                "pidsLimit": 128
            }
        }
    ]
}
```

- Success:
    - `201 Created`
- Failure:
    - `400 Bad Request` (validation/runtime/network apply failure)

**Response**

```json
{
    "sandbox": {
        "id": "sbx-http-demo",
        "phase": "running",
        "namespace": "sandbox-demo",
        "ip": "10.89.0.18",
        "subnetCIDR": "10.89.0.0/16",
        "bridgeName": "sbx-br0",
        "egress": true,
        "ports": [
            {
                "hostPort": 30080,
                "containerPort": 80,
                "protocol": "tcp"
            }
        ],
        "containers": {
            "web": {
                "id": "sbx-http-demo-web",
                "name": "web",
                "phase": "running",
                "image": "docker.io/library/nginx:alpine",
                "args": [
                    "sh",
                    "-c",
                    "nginx -g \"daemon off;\""
                ],
                "snapshotKey": "sbx-http-demo-web-snapshot",
                "taskPID": 12345,
                "runtime": "runc",
                "taskStatus": "running"
            },
            "worker": {
                "id": "sbx-http-demo-worker",
                "name": "worker",
                "phase": "running",
                "image": "docker.io/library/alpine:3.20",
                "args": ["sh", "-c", "while true; do sleep 60; done"],
                "snapshotKey": "sbx-http-demo-worker-snapshot",
                "taskPID": 12346,
                "runtime": "runc",
                "taskStatus": "running"
            }
        },
        "cniConfPath": "/etc/cni/net.d/20-sbxnet.conflist",
        "createdAt": "2026-05-06T05:00:00Z",
        "updatedAt": "2026-05-06T05:00:01Z"
    },
    "external_ip": "203.0.113.10"
}
```

### List Sandboxes

- `GET /v1/sandboxes`
- Success: `200 OK`

**Response**

```json
{
    "items": [
        {
            "id": "sbx-http-demo",
            "namespace": "sandbox-demo",
            "ip": "10.89.0.18",
            "subnetCIDR": "10.89.0.0/16",
            "bridgeName": "sbx-br0",
            "egress": true,
            "ports": [
                {
                    "hostPort": 30080,
                    "containerPort": 80,
                    "protocol": "tcp"
                }
            ],
            "containers": {
                "web": {
                    "id": "sbx-http-demo-web",
                    "name": "web",
                    "image": "docker.io/library/nginx:alpine",
                    "snapshotKey": "sbx-http-demo-web-snapshot",
                    "taskPID": 12345,
                    "runtime": "runc",
                    "taskStatus": "running"
                },
                "worker": {
                    "id": "sbx-http-demo-worker",
                    "name": "worker",
                    "image": "docker.io/library/alpine:3.20",
                    "snapshotKey": "sbx-http-demo-worker-snapshot",
                    "taskPID": 12346,
                    "runtime": "runc",
                    "taskStatus": "running"
                }
            },
            "cniConfPath": "/etc/cni/net.d/20-sbxnet.conflist",
            "createdAt": "2026-05-06T05:00:00Z"
        }
    ],
    "external_ip": "203.0.113.10"
}
```

### Get Sandbox

- `GET /v1/sandboxes/{id}`
- Success: `200 OK`
- Failure:
    - `404 Not Found` (sandbox state missing)
    - `500 Internal Server Error` (state read/runtime refresh failure)

**Response**

```json
{
    "sandbox": {
        "id": "sbx-http-demo",
        "namespace": "sandbox-demo",
        "ip": "10.89.0.18",
        "subnetCIDR": "10.89.0.0/16",
        "bridgeName": "sbx-br0",
        "egress": true,
        "ports": [
            {
                "hostPort": 30080,
                "containerPort": 80,
                "protocol": "tcp"
            }
        ],
        "containers": {
            "web": {
                "id": "sbx-http-demo-web",
                "name": "web",
                "image": "docker.io/library/nginx:alpine",
                "snapshotKey": "sbx-http-demo-web-snapshot",
                "taskPID": 12345,
                "runtime": "runc",
                "taskStatus": "running"
            },
            "worker": {
                "id": "sbx-http-demo-worker",
                "name": "worker",
                "image": "docker.io/library/alpine:3.20",
                "snapshotKey": "sbx-http-demo-worker-snapshot",
                "taskPID": 12346,
                "runtime": "runc",
                "taskStatus": "running"
            }
        },
        "cniConfPath": "/etc/cni/net.d/20-sbxnet.conflist",
        "createdAt": "2026-05-06T05:00:00Z"
    },
    "external_ip": "203.0.113.10"
}
```

### Delete Sandbox

- `DELETE /v1/sandboxes/{id}`
- Success: `200 OK`

**Response**

```json
{
    "id": "sbx-http-demo",
    "phase": "deleted",
    "external_ip": "203.0.113.10"
}
```

### Manual Reconcile

- `POST /v1/reconcile`
- Success: `200 OK`
- Failure:
    - `500 Internal Server Error`

**Response**

```json
{
    "ok": true,
    "external_ip": "203.0.113.10"
}
```

## Common Error Response

```json
{
    "error": "error message"
}
```
