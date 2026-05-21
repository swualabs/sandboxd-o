<p align="center">
  <img src="./assets/banner.png" alt="sandboxd-o">
</p>

[![codecov](https://codecov.io/github/swualabs/sandboxd-o/graph/badge.svg?token=QRJHESTOX0)](https://codecov.io/github/swualabs/sandboxd-o)
[![Backend Test CI](https://github.com/swualabs/sandboxd-o/actions/workflows/backend-test-ci.yaml/badge.svg)](https://github.com/swualabs/sandboxd-o/actions/workflows/backend-test-ci.yaml)

# sandboxd-o: Containerd CRI and gVisor shim based sandbox-like runtime, orchestrator

> [!WARNING]
> 
> This project provides a **sandbox-like** environment, but **fundamentally operates on top of container technology**. (Containers are not a [sandbox](https://en.wikipedia.org/wiki/Sandbox_(computer_security))!)
> 
> Even though gVisor offers stronger isolation, there are still limitations compared to more robust isolation mechanisms such as Firecracker Micro-VMs or KVM-based virtualization.
> 
> Therefore, this project should be used with those limitations in mind. It is recommended to deploy it in a dedicated computing environment (worker nodes) rather than running it alongside environments containing critical production data.
> 
> While the likelihood of container escape may be low, it is important to remember that container technology does not fundamentally provide a perfectly isolated sandbox environment.

**sandboxd-o**, officially named **Sandboxd-OCI**(Open Container Initiative) or **Sandboxd-Orchestrator**, is a sandbox runtime and orchestrator built on top of **Containerd CRI** and the **gVisor shim**.

This project was developed to provide an isolated environment using container technology and serves as a replacement for the previous [Container Provisioner with Kubernetes](https://github.com/nullforu/container-provisioner-k8s).

The previous project required deploying a Kubernetes cluster, introducing unnecessary overhead simply to leverage Kubernetes orchestration capabilities.
In contrast, sandboxd-o includes orchestration functionality natively and is designed as a lightweight container-based runtime focused solely on sandbox environments.

This project is actively used as a core component of the sandbox environment(VM) in [N4U Wargame](https://github.com/nullforu/wargame) and is being developed with the goal of enabling stable operation in real production environments.

# About: Technical Architecture

This project is broadly divided into three main components. Each component is described below, and the overall architecture was designed by referencing and simplifying the structure of Kubernetes.

![full architecture](./assets/full.drawio.png)

## Sandboxd Let(sbxlet) Runtime

This component corresponds to Kubernetes' kubelet and is deployed to each worker node as a daemon/agent responsible for provisioning and managing sandboxes and containers.

It uses gVisor as the container runtime to provide an isolated sandbox environment and manages sandboxes and networking based on PodSandbox from Containerd CRI.

![sbxlet Architecture](./assets/sbxlet-architecture.drawio.png)

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

![sbxlet Networking Model](./assets/sbxlet-networking-model.drawio.png)

> [!NOTE]
>
> Although `firewall` and `portmap` are included in the CNI chain, they are no longer used. Their functionality has been replaced with a model that manages forwarding and NAT using `iptables`.

The default networking configuration is shown below. These values can be customized through environment variables.

- Bridge Interface: `sbx-br0`
- IPAM CIDR: `10.89.0.0/16`

### State Management and Reconcile Loop

sbxlet stores state files to manage container runtime state, typically using the following paths:

* State storage: `/var/lib/sandboxd/sandboxes`
* Lock files: `/var/lib/sandboxd/locks`

These paths can be changed through environment variables described later, but they are the recommended defaults, and changing them arbitrarily may lead to unexpected behavior.

In addition, sbxlet maintains its own Reconcile Loop independently from the orchestrator.

The Reconcile Loop periodically compares the actual container state with the persisted state files and, when inconsistencies are detected, updates the state files or removes containers if necessary.

> This project follows the principle that sandboxes may terminate at any time and must never be recovered. Therefore, if a sandbox has terminated or its state file no longer exists, the Reconcile Loop removes the corresponding sandbox state information.

A single sbxlet instance is intended to run on a single node. Running multiple sbxlet instances against the same state storage or allowing concurrent access is not supported.

To interact with a running sbxlet instance, use the proxied sbxorch API or the `sbxctl` command.

As described later, you can specify the `--node` option to send API requests directly to the sbxlet instance running on a specific worker node.

However, this approach is not recommended, since it bypasses sbxorch features such as scheduling and the Reconcile Loop, and should only be used when absolutely necessary.

## Sandboxd Orchestrator(sbxorch) 

This is the official orchestrator of sandboxd-o and is responsible for scheduling sandboxes onto appropriate worker nodes, allocating host ports, and handling the Reconcile loop as well as the Resource/Status Sync Loop.

![full architecture](./assets/full.drawio.png)

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

<!-- TODO -->

### Node Resource Sync Loop

### Sandbox Status Sync Loop

### Scheduler Loop

### Reconcile Loop

# Installation, Build and Usage

# Environment Variables

# Testing

# Reference

# API Documentation

# FAQ, Troubleshooting and Best Practices

# Contribution and Contributors

# License
