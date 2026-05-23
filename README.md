<p align="center">
  <img src="./assets/banner.png" alt="sandboxd-o">
</p>

[![codecov](https://codecov.io/github/swualabs/sandboxd-o/graph/badge.svg?token=QRJHESTOX0)](https://codecov.io/github/swualabs/sandboxd-o)
[![Backend Test CI](https://github.com/swualabs/sandboxd-o/actions/workflows/backend-test-ci.yaml/badge.svg)](https://github.com/swualabs/sandboxd-o/actions/workflows/backend-test-ci.yaml)

# sandboxd-o: Containerd CRI and gVisor shim based sandbox-like runtime, orchestrator

> [!WARNING]
>
> This project provides a **sandbox-like** environment, but **fundamentally operates on top of container technology**. (Containers are not a [sandbox](<https://en.wikipedia.org/wiki/Sandbox_(computer_security)>)!)
>
> Even though gVisor offers stronger isolation, there are still limitations compared to more robust isolation mechanisms such as Firecracker Micro-VMs or KVM-based virtualization.
>
> Therefore, this project should be used with those limitations in mind. It is recommended to deploy it in a dedicated computing environment (worker nodes) rather than running it alongside environments containing critical production data.
>
> While the likelihood of container escape may be low, it is important to remember that container technology does not fundamentally provide a perfectly isolated sandbox environment.

---

- [Overview](#overview)
- [About: Technical Architecture](#about-technical-architecture)
    - [Sandboxd Let(sbxlet) Runtime](#sandboxd-letsbxlet-runtime)
        - [Networking Model](#networking-model)
        - [State Management and Reconcile Loop](#state-management-and-reconcile-loop)
    - [Sandboxd Orchestrator(sbxorch)](#sandboxd-orchestratorsbxorch)
        - [HTTP Server and Database](#http-server-and-database)
        - [Sync Loops](#sync-loops)
        - [Scheduler/Reconcile Loops](#schedulerreconcile-loops)
    - [Sandboxd CLI(sbxctl)](#sandboxd-clisbxctl)
- [Resource Model/Objects](#resource-modelobjects)
    - [Node](#node)
    - [External](#external)
    - [Sandbox](#sandbox)
        - [egress, ttl_seconds, ports](#egress-ttl_seconds-ports)
        - [containers](#containers)
- [Sandbox/Container State](#sandboxcontainer-state)
- [Installation, Build and Usage](#installation-build-and-usage)
    - [Requirements](#requirements)
    - [Installation Runtime Dependencies](#installation-runtime-dependencies)
    - [Build and Usage](#build-and-usage)
- [Environment Variables](#environment-variables)
- [Testing](#testing)
- [Performance and Benchmarking](#performance-and-benchmarking)
- [Reference](#reference)
- [API Documentation](#api-documentation)
- [FAQ, Troubleshooting and Best Practices](#faq-troubleshooting-and-best-practices)
- [Contribution and Contributors](#contribution-and-contributors)
- [License](#license)

---

# Overview

**sandboxd-o**, officially named **Sandboxd-OCI**(Open Container Initiative) or **Sandboxd-Orchestrator**, is a sandbox runtime and orchestrator built on top of **Containerd CRI** and the **gVisor shim**.

This project was developed to provide an isolated environment using container technology and serves as a replacement for the previous [Container Provisioner with Kubernetes](https://github.com/nullforu/container-provisioner-k8s).

The previous project required deploying a Kubernetes cluster, introducing unnecessary overhead simply to leverage Kubernetes orchestration capabilities.
In contrast, sandboxd-o includes orchestration functionality natively and is designed as a lightweight container-based runtime focused solely on sandbox environments.

This project is actively used as a core component of the sandbox environment(VM) in [N4U Wargame](https://github.com/nullforu/wargame) and is being developed with the goal of enabling stable operation in real production environments.

# About: Technical Architecture

This project is broadly divided into three main components. Each component is described below, and the overall architecture was designed by referencing and simplifying the structure of Kubernetes.

![full architecture](./assets/full.png)

## Sandboxd Let(sbxlet) Runtime

This component corresponds to Kubernetes' kubelet and is deployed to each worker node as a daemon/agent responsible for provisioning and managing sandboxes and containers.

It uses gVisor as the container runtime to provide an isolated sandbox environment and manages sandboxes and networking based on PodSandbox from Containerd CRI.

![sbxlet Architecture](./assets/sbxlet-architecture.png)

To achieve the project's core goal of providing "the strongest possible isolation within practical limits," sandbox environments are built using `runsc`, the runtime of gVisor.

> [!NOTE]
>
> gVisor is a technology that strengthens isolation between containers and the host system by emulating the Linux kernel. It intercepts and handles system calls through a user-space kernel called Sentry. This prevents containers from directly interacting with the host system and improves security.
>
> In addition, gVisor uses a user-space filesystem component called Gofer to mediate container filesystem access, preventing containers from directly accessing the host filesystem.

The filesystem surface can leverage `overlayfs` to provide a sandbox filesystem isolated from the host filesystem.

This project follows the principle that once a sandbox is created, it must be treated as disposable and single-use. As a result, sharing or persisting filesystems (volumes) is not supported.

### Networking Model

Another core responsibility of sbxlet is its networking model. Internally, it uses the bridge and loopback CNI (Container Network Interface) plugins to configure bridge networking and loopback networking.

Additionally, it leverages host-local IPAM (Host-local IP Address Management) to allocate and manage IP addresses for each sandbox.

Using host-local IPAM, each sandbox is assigned a unique private IP address. The loopback CNI provides localhost networking inside the sandbox, while the bridge CNI enables communication between sandboxes.

Traffic routing is then handled using `iptables` to support host-to-sandbox access and external traffic entering through the host NIC, including forwarding and NAT (Network Address Translation).

For a more detailed flow, please refer to the architecture diagram below.

![sbxlet Networking Model](./assets/sbxlet-networking-model.png)

> [!NOTE]
>
> Although `firewall` and `portmap` are included in the CNI chain, they are no longer used. Their functionality has been replaced with a model that manages forwarding and NAT using `iptables`.

The default networking configuration is shown below. These values can be customized through environment variables.

- Bridge Interface: `sbx-br0`
- IPAM CIDR: `10.89.0.0/16`

### State Management and Reconcile Loop

sbxlet stores state files to manage container runtime state, typically using the following paths:

- State storage: `/var/lib/sandboxd/sandboxes`
- Lock files: `/var/lib/sandboxd/locks`

These paths can be changed through environment variables described later, but they are the recommended defaults, and changing them arbitrarily may lead to unexpected behavior.

In addition, sbxlet maintains its own Reconcile Loop independently from the orchestrator.

The Reconcile Loop periodically compares the actual container state with the persisted state files and, when inconsistencies are detected, updates the state files or removes containers if necessary.

> [!NOTE]
>
> This project follows the principle that sandboxes may terminate at any time and must never be recovered. Therefore, if a sandbox has terminated or its state file no longer exists, the Reconcile Loop removes the corresponding sandbox state information.

A single sbxlet instance is intended to run on a single node. Running multiple sbxlet instances against the same state storage or allowing concurrent access is not supported.

To interact with a running sbxlet instance, use the proxied sbxorch API or the `sbxctl` command.

As described later, you can specify the `--node` option to send API requests directly to the sbxlet instance running on a specific worker node.

However, this approach is not recommended, since it bypasses sbxorch features such as scheduling and the Reconcile Loop, and should only be used when absolutely necessary.

## Sandboxd Orchestrator(sbxorch)

This is the official orchestrator of sandboxd-o and is responsible for scheduling sandboxes onto appropriate worker nodes, allocating host ports, and handling the Reconcile loop as well as the Resource/Status Sync Loop.

![full architecture](./assets/full.png)

### HTTP Server and Database

This component is an HTTP API server that receives and handles client requests. It also includes Admission logic for validating resource specifications (Specs). For more details about the API, refer to the [API documentation](./docs/orchestrator.md) or the Swagger documentation.

The sbxorch orchestrator supports proxied APIs that allow requests to target a specific worker node (more precisely, a specific sbxlet instance). These endpoints are available under `/api/v1/nodes/:id/...`. For additional details, refer to the orchestrator API documentation or the [sbxlet API](./docs/sandboxd.md) documentation.

The storage backend used by sbxorch to manage objects and resources is SQLite by default.

SQLite was chosen because it is a lightweight file-based database, and distributed sbxorch API server deployments are not currently within the project's scope.

By default, the database is created at `/var/lib/sandboxd/orchestrator.db` and acts as the Source of Truth for managing sandbox resources as well as resources such as Node and External.

> [!NOTE]
>
> etcd was considered during the early stages of development, but was deferred because the operational overhead and complexity were not considered appropriate for the scale of this project.
>
> Additionally, while the current implementation uses relational structures with tightly connected entities, there are plans to redesign the architecture in the future around a key-value model for a faster, simpler, and more hierarchical structure.

### Sync Loops

sbxorch internally maintains several Sync Loops to preserve system consistency and stability between sbxlet and sbxorch. The major Sync Loops are as follows:

- **Node Resource Sync Loop**: Periodically retrieves node resource status from sbxlet and stores it in the database. This information is used during scheduling to score nodes and place sandboxes onto appropriate worker nodes. Although resource allocation changes are applied logically when scheduling or removing nodes, the final resource state is reconciled and updated by this loop.

- **Sandbox Status Sync Loop**: Periodically retrieves sandbox status from sbxlet and stores it in the database. This allows sbxorch to track the state of each sandbox and detect failures, runtime issues in sbxlet, errors inside the sandbox, or other state changes, enabling it to take appropriate actions.

> [!NOTE]
>
> The interval of these loops can be adjusted through environment variables, and configuring appropriate intervals is recommended for production environments.

### Scheduler/Reconcile Loops

sbxorch generally aims to follow a model similar to Kubernetes' declarative architecture, and to support this, it includes both a Scheduler Loop and a Reconcile Loop.

The **Scheduler Loop** is responsible for detecting sandbox resources that require scheduling and assigning them to appropriate worker nodes.

When a Sandbox is created, it initially enters the `Pending` state (see below for details), which indicates that scheduling is required. The Scheduler Loop continuously detects these pending sandboxes and schedules them onto suitable nodes.

The **Reconcile Loop** periodically compares the actual system state with the resource state stored in the database. When inconsistencies are detected, it updates resource states or removes resources if necessary.

However, following this project's principle that sandboxes may terminate at any time and must not be recovered, the Reconcile Loop is currently used only as a TTL (Time To Live)-based sandbox resource cleanup mechanism rather than performing resource restoration.

> [!NOTE]
>
> The intervals for both of these loops can also be configured through environment variables, and appropriate values are recommended for production environments.

```json
{"ts":"2026-05-22T18:13:13.560811039+09:00","level":"info","msg":"scheduler tick completed","app":"orchestrator:dev","sandbox_total":0,"pending_count":0}
{"ts":"2026-05-22T18:13:15.559090911+09:00","level":"info","msg":"reconcile tick completed","app":"orchestrator:dev","sandbox_total":0,"deleting_count":0,"expired_count":0}
```

## Sandboxd CLI(sbxctl)

This is a CLI tool equivalent to Kubernetes' `kubectl`, used to communicate with the sbxorch API server to manage resource models and inspect system state.

```
sandboxd control client

Usage:
  sbxctl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create resource from YAML file
  delete      Delete resource
  get         Get resources
  help        Help about any command
  logs        Get container logs via orchestrator node proxy
  spec        Print resource in YAML spec form

Flags:
  -h, --help               help for sbxctl
      --limit int          log/list limit (default 100)
      --node string        node id for proxy APIs
  -o, --output string      output format: json|yaml|wide
      --server string      orchestrator base url (or SBXCTL_SERVER)
      --timeout duration   request timeout (default 10s)

Use "sbxctl [command] --help" for more information about a command.
```

The main available options are as follows:

- `--server`: Specifies the base URL of the API server. This option can also be configured through the `SBXCTL_SERVER` environment variable. For example: `http://localhost:8080`
- `--node`: Specifies the target node ID when using the Proxied API. This allows API requests to be sent directly to a specific sbxlet instance. For example: `sandboxd-node-1`
- `-o, --output`: Specifies the output format. Available options are `json`, `yaml`, and `wide`. Table output is used by default.

The main available commands are as follows:

- `get`: Used to retrieve a list of resources or inspect detailed information for a specific resource. Resource types may be referenced using either their full names (`nodes`, `external`, `sandboxes`) or shorthand forms (`n`, `e`, `s`). Detailed inspection of a resource can be performed using `/` separators.
- `create`: Used to create resources from a YAML file. For example: `sbxctl create -f examples/node.yaml`
- `delete`: Used to delete resources. For example: `sbxctl delete node/sandboxd-node-1`. A specific resource must be explicitly specified.
- `logs`: Used to retrieve logs from containers inside a sandbox. For example: `sbxctl logs s/sbx-wordpress-demo wordpress`. Internally, this command uses sbxorch's Proxied API to send requests directly to the sbxlet instance running on the target node and retrieve logs.

# Resource Model / Objects

The resource model in this project is designed to resemble Kubernetes' resource model but is simplified to better fit sandbox environments.

At this stage, advanced objects such as Deployment or DaemonSet, as well as concepts like Controllers and reconciliation-based resource management, are intentionally excluded.

The following objects are currently available:

- `sandboxd.o/v1` - `Node`
- `sandboxd.o/v1` - `External`
- `sandboxd.o/v1` - `Sandbox`

> [!NOTE]
>
> API groups exist for structural and compatibility purposes, but they do not currently provide any separate functionality.

## Node

A Node represents a worker node onto which sandboxes can be scheduled.

It is a resource used to register an installed sbxlet instance with sbxorch and contains the IP address and port information of the corresponding sbxlet instance.

```yaml
# examples/node.yaml

apiVersion: sandboxd.o/v1
kind: Node
id: sandboxd-node-1
spec:
    ip: '127.0.0.1'
    port: 8081
```

```shell
> sbxctl create -f examples/node.yaml
> sbxctl get nodes -o wide
RESOURCE: node
NAME             STATE  IP         PORT  EXTERNAL       CPU(ALLOC/USED/AVAIL)  MEM(ALLOC/USED/AVAIL)  LAST_ERROR  HEARTBEAT                       UPDATED
sandboxd-node-1  Ready  127.0.0.1  8081  host1.swua.kr  3600m/0m/3600m         14342MB/0MB/14342MB                2026-05-22T09:17:40.566923735Z  2026-05-21T02:54:11.293035905Z
```

Node status can be one of `Ready`, `NotReady`, or `Unknown`, and sandboxes can only be scheduled onto Nodes that are in the `Ready` state.

- The `Unknown` state occurs when a node has been newly registered and no Heartbeat has been received yet, or when the Threshold or Grace Period has not elapsed sufficiently to determine its status.
- The `NotReady` state occurs when a node has not sent a Heartbeat for a certain period of time, or when a Heartbeat was received but the Threshold or Grace Period conditions were not satisfied for the node to be considered `Ready`.

## External

The External object is a resource used to register a Public IP address or hostname (domain) for a node.

It does not participate in internal networking behavior, but is used as a logical representation of an externally reachable endpoint associated with a node.

```yaml
# examples/external.yaml

apiVersion: sandboxd.o/v1
kind: External
id: sandboxd-node-external-1
spec:
    node_id: 'sandboxd-node-1'
    external: 'host1.swua.kr'
```

The `node_id` field specifies the ID of the Node referenced by this External resource, and the `external` field represents the externally reachable endpoint associated with that node.

If this object is not configured, the accessible Public IP address or hostname for that node will be displayed as `(none)`.

```shell
> sbxctl get external
RESOURCE: external
ID                        NODE_ID          EXTERNAL       UPDATED
sandboxd-node-external-1  sandboxd-node-1  host1.swua.kr  2026-05-21T02:54:12.778649884Z
```

## Sandbox

A Sandbox is a resource that represents an isolated sandbox environment.

It represents a sandbox environment that is provisioned and managed by sbxlet. The following example defines a sandbox environment containing both WordPress and MySQL.

```yaml
# examples/wordpress.yaml

apiVersion: sandboxd.o/v1
kind: Sandbox
id: sbx-wordpress-demo
spec:
    egress: true
    ttl_seconds: 3600
    ports:
        - container_port: 80
          protocol: tcp
    containers:
        - name: wordpress
          image: wordpress:6.9.4-php8.3-apache
          args:
              - sh
              - -c
              - >-
                  for i in $(seq 1 90); do php -r
                  '$s=@fsockopen("127.0.0.1",3306,$e,$es,1);
                  if($s){fclose($s); exit(0);} exit(1);'
                  && break; sleep 2; done;
                  exec docker-entrypoint.sh apache2-foreground
          env:
              - WORDPRESS_DB_HOST=127.0.0.1:3306
              - WORDPRESS_DB_USER=wordpress
              - WORDPRESS_DB_PASSWORD=wordpress-pass
              - WORDPRESS_DB_NAME=wordpress
          work_dir: ''
          resource:
              cpu: 1000m
              memory: 1024Mi
        - name: mysql
          image: mysql:8.4
          args: []
          env:
              - MYSQL_DATABASE=wordpress
              - MYSQL_USER=wordpress
              - MYSQL_PASSWORD=wordpress-pass
              - MYSQL_ROOT_PASSWORD=root-pass
          work_dir: ''
          resource:
              cpu: 1000m
              memory: 1024Mi
```

The description of each field is as follows.

### egress, ttl_seconds, ports

- `egress`: Determines whether this sandbox is allowed to send outbound traffic to external networks. When set to `true`, outbound traffic from the sandbox to external networks is allowed. When set to `false`, outbound traffic is blocked. This behavior is enforced by sbxlet's networking model.

- `ttl_seconds`: The amount of time (in seconds) before the sandbox is automatically terminated after creation. If this value is `0` or not specified, the sandbox continues running until it is manually deleted. If a positive value is specified, the sandbox is automatically terminated after the configured duration has elapsed. This mechanism is implemented through the Reconcile Loop, which detects and terminates sandboxes whose TTL has expired.

- `ports`: The list of ports to expose from this sandbox. Each port consists of `container_port` and `protocol`. `container_port` specifies the port number exposed inside the sandbox, while `protocol` specifies the protocol used for that port. Currently, `tcp` and `udp` are supported. This field is optional, and if omitted, no ports are exposed.

    Direct Host Port assignment is intentionally not supported because sbxorch maintains and allocates its own Host Port Pool. Following the project's philosophy of exposing large numbers of Host Ports to arbitrary users, manual Host Port assignment has been intentionally disallowed.

> [!WARNING]
>
> `egress: false` can be useful in scenarios such as:
>
> - Intentionally preventing participants in CTF/Wargame VM environments from invoking external webhooks
> - Running sandboxes in environments where outbound access to external networks must be prohibited due to security policies or internal governance requirements
>
> However, disabling this option blocks all outbound traffic, meaning operations such as `apt-get` or application-level external API calls will no longer work.
>
> In addition, some DNS resolvers may not function properly, so you should ensure that all required packages and applications are already included in the container image and verify that the application can operate correctly without outbound network access.

Therefore, in the `wordpress.yaml` example above, the sandbox is automatically terminated by the Reconcile Loop after one hour (3600 seconds) from creation, and TCP port `80` is exposed inside the sandbox.

Additionally, since `egress` is set to `true`, outbound traffic to external networks is permitted for this sandbox.

### containers

Sandbox fundamentally follows and is built upon the PodSandbox model, and therefore consists of a Pause Container and one or more Application Containers.

The `containers` field represents the list of Application Containers included in this sandbox.

Each container is composed of `name`, `image`, `args`, `env`, `work_dir`, and `resource`.

The following is an example container specification capable of running a sample application.

```yaml
- name: my-app
  image: my-app-image:latest
  args: []
  env:
      - SECRET_KEY=supersecretkey
  work_dir: '/app'
  resource:
      cpu: 100m
      memory: 256Mi
      ephemeral_storage: 96Mi
```

- `name`: The name of the container. It must be unique within the sandbox. Rather than generating it randomly, the name is explicitly specified so that it can be configured directly by the client.

- `image`: The container image. This must be an OCI (Open Container Initiative)-compatible image reference. For example: `my-app-image:latest` or `docker.io/library/my-app-image:latest`. The implementation first attempts to use an image already available on the node, and only pulls from the registry if the image does not exist locally.

- `args`: The command and argument list executed when the container starts. For example: `["sh", "-c", "echo Hello World"]`. This field is optional, and if omitted, the container starts using the image's default command and arguments.

- `env`: A list of environment variables available inside the container. Each environment variable is represented as a string in `KEY=VALUE` format. This field is optional, and if omitted, no additional environment variables are injected.

- `work_dir`: The working directory of the container. The container switches to this directory at startup. This field is optional, and if omitted, the container uses its default working directory.

- `resource`: The resources allocated to the container. While Kubernetes typically uses a Request/Limit model, this project simplifies resource allocation by using a single resource model consisting of `cpu` and `memory`.

    In other words, this behaves similarly to Kubernetes' Guaranteed QoS model.

    `cpu` represents the CPU resources allocated to the container, and `memory` represents the memory resources allocated to the container. These values act as both Request and Limit and determine the maximum amount of resources the container can consume.

    `ephemeral_storage` is optional and controls writable filesystem limits. When set, sbxlet applies this total budget by splitting it into:
    - root writable layer (`/`) = 80%
    - temporary filesystem (`/tmp`) = 20%

    The root writable layer and `/tmp` will return `ENOSPC` when each split limit is exceeded.

    CPU values are expressed in millicores, for example `100m` represents `0.1` CPU.

    Memory values are expressed in bytes, for example `256Mi` represents 256 mebibytes (approximately 268,435,456 bytes).

    Resource enforcement is implemented using cgroups, and gVisor is custom patched, built, and used for proper enforcement.

    For more details, refer to the [Reference](#reference) section below.

> [!NOTE]
> However, in the case of `/dev/shm`, it appears that `runsc` internally mounts a separate `tmpfs`.
>
> As a result, it could not be controlled at the code level in the current implementation. Further investigation and experimentation will be conducted in the future to determine whether limits can be enforced for this mount point as well.

## Sandbox/Container State

In sbxlet, sandboxes and containers have the following states, and each state is mapped to the corresponding sbxorch sandbox state.

- **sbxlet Sandbox states**: `Creating`, `Running`, `Deleting`, `Error`
- **sbxlet Container states**: `Creating`, `Running`, `Stopped`, `Error`, `Unknown`

The state of an sbxlet sandbox transitions to `Running` once all containers reach the `Running` state.

If any container enters the `Error` state, the sandbox transitions to `Error`. In this case, the error message is recorded in the Last Error field.

Additionally, when a sandbox is being removed, its state transitions to `Deleting`.

- **sbxorch Sandbox states**: `Pending`, `Scheduled`, `Running`, `Failed`, `Deleting`

The sandbox state in sbxorch is determined by mapping the corresponding sandbox state from sbxlet.

A sandbox is initially created in the `Pending` state, then scheduled and assigned to a node. Once the sandbox managed by sbxlet on that node reaches the `Running` state, the sandbox state in sbxorch also transitions to `Running`.

If the sandbox enters the `Error` state in sbxlet, the corresponding sbxorch sandbox transitions to `Failed`.

Likewise, when a sandbox is deleted through sbxorch, its state transitions to `Deleting`.

> [!NOTE]
>
> Even when a sandbox is in the `Running` state, it may not actually be fully ready due to initialization tasks such as image preparation, network configuration, or application startup inside the container.
>
> Therefore, a sandbox reaching the `Running` state should not be assumed to mean that it is immediately usable. When necessary, it is recommended to implement an external readiness mechanism, similar to a Readiness Probe, to verify that the sandbox is actually ready.
>
> However, this project does not provide a built-in Readiness Probe mechanism. Instead, as demonstrated in the `wordpress.yaml` example above, the recommended approach is to implement waiting logic through custom scripts in the container's `args` field so that execution proceeds only after the application becomes fully ready.

This information can be checked using commands such as `sbxctl get sandboxes` or `sbxctl get sandboxes/:ID`, or through the API.

```shell
> sbxctl get s -o wide
RESOURCE: sandbox
NAME                PHASE    NODE             EXTERNAL       IP          PORTS          EGRESS  CONTAINERS  EXPIRE_AT                       LAST_ERROR  CREATED
sbx-wordpress-demo  Running  sandboxd-node-1  host1.swua.kr  10.89.1.35  22761/tcp->80  true    2           2026-05-22T10:45:58.249474717Z              2026-05-22T09:45:58.249474717Z

> sbxctl get s/sbx-wordpress-demo -o yaml
sandbox:
    created_at: "2026-05-22T09:45:58.249474717Z"
    id: sbx-wordpress-demo
    spec:
        containers:
            - args:
                - sh
                - -c
                - for i in $(seq 1 90); do php -r '$s=@fsockopen("127.0.0.1",3306,$e,$es,1); if($s){fclose($s); exit(0);} exit(1);' && break; sleep 2; done; exec docker-entrypoint.sh apache2-foreground
              env:
                - WORDPRESS_DB_HOST=127.0.0.1:3306
                - WORDPRESS_DB_USER=wordpress
                - WORDPRESS_DB_PASSWORD=wordpress-pass
                - WORDPRESS_DB_NAME=wordpress
              image: wordpress:6.9.4-php8.3-apache
              name: wordpress
              resource:
                cpu: 1000m
                memory: 1024Mi
            - env:
                - MYSQL_DATABASE=wordpress
                - MYSQL_USER=wordpress
                - MYSQL_PASSWORD=wordpress-pass
                - MYSQL_ROOT_PASSWORD=root-pass
              image: mysql:8.4
              name: mysql
              resource:
                cpu: 1000m
                memory: 1024Mi
        egress: true
        ports:
            - container_port: 80
              protocol: tcp
        ttl_seconds: 3600
    status:
        assigned_ports:
            - container_port: 80
              host_port: 22761
              protocol: tcp
        expire_at: "2026-05-22T10:45:58.249474717Z"
        external: host1.swua.kr
        ip: 10.89.1.35
        node_name: sandboxd-node-1
        phase: Running
    updated_at: "2026-05-22T09:46:10.588422069Z"
```

> [!NOTE]
>
> The final resource allocation of a sandbox is determined as the sum of the resources allocated to its containers. Therefore, in the example above, the sandbox is calculated to use **2 vCPUs (2000m)** and **2048Mi of memory**.

# Installation, Build and Usage

## Requirements

- **x86-64 architecture**
- **Ubuntu 24.04 LTS or later** (tested on the AWS EC2 Ubuntu 24.04 AMI)
- Go 1.25.5 or later
- Other binary versions can be checked in the install script, and all components are installed automatically.

> [!NOTE]
>
> ARM architecture is not officially supported. It may be built and used separately, but it is not officially tested or supported.

- ...and **it does not require virtualization technologies such as KVM, QEMU, or a hypervisor!** This project is built on **container technology**. While it requires the kernel version and features needed by gVisor's `runsc` runtime, it does not require separate virtualization technology.

## Installation Runtime Dependencies

This project requires runtime dependencies such as Containerd, CRI Plugins, and gVisor (`runsc`), and additionally requires CNI plugins to be configured.

It also requires runtime environment setup such as enabling certain kernel options (`bridge-nf-call-iptables`, etc.) and registering systemd services.

Additionally, the gVisor runtime shim used by this project is a custom-patched version, so the patched binary is automatically included and installed.

An installation script covering these steps is provided at [scripts/install.sh](./scripts/install.sh), and runtime dependencies can be installed using that script.

```shell
chmod +x scripts/install.sh
sudo ./scripts/install.sh
```

> [!NOTE]
>
> The installation script must be executed with sudo privileges, and it has been tested on x86-64 architecture and Ubuntu 24.04 LTS or later.

## Build and Usage

At the moment, this project can either be built directly from source or used by downloading prebuilt binaries from releases.

The following example demonstrates how to clone the source code via git and build it locally.

```shell
git clone https://github.com/swualabs/sandboxd-o.git
cd sandboxd-o

cp .env.example .env
# vi .env

make build
```

After building, the `sbxlet`, `sbxorch`, and `sbxctl` binaries will be generated under the `./build` directory.

You can start sbxlet and sbxorch by executing these binaries with sudo privileges.

The following script demonstrates one example of running sbxlet and sbxorch in the background. (Since log files are stored under `/logs/sbxlet` and `/logs/sbxorch` respectively, it is safe to redirect the binaries' stdout/stderr to `/dev/null`.)

```shell
sudo ./build/sbxlet > /dev/null 2>&1 &
sudo ./build/sbxorch > /dev/null 2>&1 &
```

More detailed installation and usage guides will be provided in the future.

# Environment Variables

Currently, the environment variables used by sbxlet, sbxorch, and sbxctl are consolidated into `.env.example` within the codebase, but the environment variables themselves are not shared across components.

## sbxlet

```ini
# Common
APP_ENV=dev # application environment, can be set to dev, staging, or prod for different logging levels and configurations

# HTTP listen address for sandboxd
HTTP_ADDR=:8081

# containerd socket
SANDBOX_CONTAINERD_ADDRESS=/run/containerd/containerd.sock # default containerd socket path, can be changed if using a custom containerd setup
SANDBOX_RUNTIME_BINARY=runsc # gVisor runtime binary, default is runsc, can be changed if using a custom built gVisor runtime
SANDBOX_CNI_CONF_PATH=/etc/cni/sandboxd.d/20-sbxnet.conflist # CNI configuration file path, can be changed if using a custom CNI setup
SANDBOX_LOG_DIR=/logs/sbxlet # log directory for sbxlet, default is /logs/sbxlet
SANDBOX_LOG_FILE_PREFIX=sandboxd # log file prefix for sbxlet, default is sandboxd

# local state/lock directories
SANDBOX_STATE_BASE_DIR=/var/lib/sandboxd/sandboxes # base directory for sandbox state files, default is /var/lib/sandboxd/sandboxes
SANDBOX_LOCK_DIR=/var/lib/sandboxd/locks # directory for lock files, default is /var/lib/sandboxd/locks

# sandbox bridge/subnet
SANDBOX_BRIDGE_INTERFACE=sbx-br0 # bridge interface name for sandbox networking, default is sbx-br0
SANDBOX_SUBNET_CIDR=10.89.0.0/16 # subnet CIDR for sandbox networking, default is 10.89.0.0/16
SANDBOX_MAX_ALLOC_PERCENT=90 # maximum percentage of node resources that can be allocated to sandboxes, default is 90% (ex. if node has 4 vCPUs and 8GB memory, up to 3.6 vCPUs and 7.2GB memory can be allocated to sandboxes)
SANDBOX_PROVISION_TIMEOUT=4m # timeout for provisioning a sandbox, including image pulling and container creation, default is 4 minutes
SANDBOX_CONTAINER_CREATE_TIMEOUT=2m # timeout for creating a container, default is 2 minutes
SANDBOX_IMAGE_PULL_TIMEOUT=8m # timeout for pulling an image, default is 8 minutes
SANDBOX_DEFAULT_EPHEMERAL_STORAGE=128Mi # default per-container writable storage budget if ephemeral_storage is omitted
SANDBOX_EPHEMERAL_ROOTFS_PERCENT=80 # split ratio for root writable layer (/), must satisfy rootfs+tmp=100
SANDBOX_EPHEMERAL_TMP_PERCENT=20 # split ratio for /tmp, must satisfy rootfs+tmp=100

# Chains to hook SANDBOX-FWD jump (comma-separated)
SANDBOX_FORWARD_HOOK_CHAINS=FORWARD,DOCKER-USER # default chains to hook iptables rules for sandbox port forwarding, can be customized as needed
```

## sbxorch

```ini
# Common
APP_ENV=dev # application environment, can be set to dev, staging, or prod for different logging levels and configurations

# Http listen address for sbxorch
ORCH_HTTP_ADDR=:8082

# Paths and files
ORCH_CONFIG_PATH=configs/apiserver.yaml # path to the orchestrator configuration file, default is configs/apiserver.yaml
ORCH_SQLITE_PATH=/var/lib/sandboxd/orchestrator.db # path to the SQLite database file, default is /var/lib/sandboxd/orchestrator.db
ORCH_LOG_DIR=/logs/sbxorch # log directory for sbxorch, default is /logs/sbxorch
ORCH_LOG_FILE_PREFIX=orchestrator # log file prefix for sbxorch, default is orchestrator

# Heartbeat / node state
ORCH_HEARTBEAT_INTERVAL=10s # interval for sending heartbeat signals to nodes, default is 10 seconds
ORCH_NODE_PROBE_TIMEOUT=3s # timeout for probing node status during heartbeat, default is 3 seconds
ORCH_SANDBOX_OP_TIMEOUT=60s # timeout for sandbox operations such as creation and deletion, default is 60 seconds
ORCH_HEARTBEAT_PARALLEL=false # whether to perform heartbeat probes in parallel, default is false (sequential)
ORCH_HEARTBEAT_MAX_PARALLEL=4 # maximum number of parallel heartbeat probes when ORCH_HEARTBEAT_PARALLEL is true, default is 4
ORCH_READY_SUCCESS_THRESHOLD=2 # number of consecutive successful heartbeats required for a node to be considered Ready, default is 2
ORCH_NOTREADY_FAILURE_THRESHOLD=2 # number of consecutive failed heartbeats required for a node to be considered NotReady, default is 2

# Resource sync / persistence
ORCH_RESOURCE_SYNC_INTERVAL=30s # interval for syncing node resources from sbxlet, default is 30 seconds
ORCH_RESOURCE_PERSIST_MIN_INTERVAL=30s # minimum interval for persisting resource state to the database, default is 30 seconds
ORCH_RESOURCE_PERSIST_MAX_INTERVAL=5m # maximum interval for persisting resource state to the database, default is 5 minutes

# Scheduler / reconcile
ORCH_SCHEDULER_INTERVAL=3s # interval for running the scheduler loop to detect and schedule pending sandboxes, default is 3 seconds
ORCH_RECONCILE_INTERVAL=5s # interval for running the reconcile loop to detect and clean up expired sandboxes, default is 5 seconds
ORCH_STATUS_SYNC_INTERVAL=20s # interval for syncing sandbox status from sbxlet, default is 20 seconds
ORCH_STATUS_SYNC_TIMEOUT=5s # timeout for syncing sandbox status from sbxlet, default is 5 seconds
ORCH_STATUS_SYNC_BATCH_SIZE=50 # number of sandboxes to sync status for in each batch during the status sync loop, default is 50
ORCH_STATUS_SYNC_MAX_PARALLEL=4 # maximum number of parallel status sync operations when syncing sandbox status from sbxlet, default is 4
ORCH_HOSTPORT_MIN=10000 # minimum host port number for sandbox port forwarding, default is 10000
ORCH_HOSTPORT_MAX=32767 # maximum host port number for sandbox port forwarding, default is 32767

# API create rate limit
ORCH_CREATE_RPS=20 # rate limit for creating sandboxes through the API, default is 20 requests per second
ORCH_CREATE_BURST=40 # burst limit for creating sandboxes through the API, default is 40 requests

ORCH_SHUTDOWN_TIMEOUT=5s # timeout for graceful shutdown of the orchestrator, default is 5 seconds
```

## sbxctl

```ini
SBXCTL_SERVER=http://10.10.0.1:8082 # server address for sbxctl to connect to the sbxorch API server. This can also be specified using the --server flag when running sbxctl commands.
```

# Testing

[![codecov](https://codecov.io/github/swualabs/sandboxd-o/graph/badge.svg?token=QRJHESTOX0)](https://codecov.io/github/swualabs/sandboxd-o)

```shell
make test
# make test-cover
```

The target test coverage is at least **70%** for both overall and PATCH coverage.

However, since there are components that are difficult to test—such as sbxlet—certain exceptions are defined. Please refer to [`codecov.yaml`](./codecov.yaml) for details.

# Performance and Benchmarking

In this section, we briefly measure the time taken at each stage of sandbox provisioning and benchmark the results against [`container-provisioner-k8s`](https://github.com/swualabs/container-provisioner-k8s), which was previously used in the [SMCTF](https://github.com/nullforu/smctf)/[N4U Wargame](https://github.com/nullforu/wargame) platform.

```
TODO
```

# Reference

- [swualabs/gvisor-shim-patched](https://github.com/swualabs/gvisor-shim-patched) : This is a patched version of the gVisor shim modified to resolve issues related to cgroup resource limit enforcement. for more details, refer to commit [`041e28d`](https://github.com/swualabs/gvisor-shim-patched/commit/041e28d) in the corresponding fork repository or [swualabs/sandboxd-o PR #11](https://github.com/swualabs/sandboxd-o/pull/11).

# API Documentation

REST API documentation can be found in [docs/orchestrator.md](./docs/orchestrator.md) or [docs/sandboxd.md](./docs/sandboxd.md), and Swagger documentation is available at the following endpoints:

- **sbxorch API Swagger Documentation**: `http://<sbxorch-address>/swagger/index.html`
- **sbxlet API Swagger Documentation**: `http://<sbxlet-address>/swagger/index.html`

# FAQ, Troubleshooting and Best Practices

# Contribution and Contributors

| Name          | GitHub                               | Role               |
| ------------- | ------------------------------------ | ------------------ |
| Kim Jun Young | [@yulmwu](https://github.com/yulmwu) | Author, Maintainer |
| ...           | ...                                  | ...                |

# MIT License

```
Copyright (c) 2026 The Swua Labs Authors, Kim Jun Young as @yulmwu

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the “Software”), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```
