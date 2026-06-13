---
status: implementable
stage: alpha
---

# VPC API Reference

**API Group:** `vpc.miloapis.com/v1alpha1`
**Stability:** Alpha — fields and defaults may change without deprecation notice

---

## 1. Overview

The VPC API defines tenant network isolation primitives for the Datum.net fleet. A VPC (Virtual Private Cloud) represents an isolated L3 network domain. Workloads are attached to a VPC via VPCAttachment resources, which bind a network interface to the VPC and assign addresses.

### API Group

| Group | Purpose |
|-------|---------|
| `vpc.miloapis.com/v1alpha1` | VPC lifecycle and workload attachment |

All resources in this group are **namespace-scoped**.

---

## 2. CRD Reference

| Kind | API Group | Short Name |
|------|-----------|------------|
| [VPC](#vpc) | `vpc.miloapis.com/v1alpha1` | — |
| [VPCAttachment](#vpcattachment) | `vpc.miloapis.com/v1alpha1` | — |

---

### VPC

`VPC` represents an isolated L3 network domain. It holds the set of CIDR blocks that define the address space for the VPC. Workloads join the VPC by creating VPCAttachment resources that reference it.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `networks` | `[]string` | Yes | CIDR blocks (IPv4 or IPv6) that define the VPC address space. Minimum 1. |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | `bool` | `true` when the VPC is provisioned and ready for attachments. |
| `identifier` | `string` | Platform-assigned unique identifier for this VPC. |

---

### VPCAttachment

`VPCAttachment` binds a network interface on a workload to a VPC. It specifies the VPC to attach to, the interface name, and the addresses assigned to that interface within the VPC.

The annotation `k8s.v1alpha1.vpc.miloapis.com/vpc-attachment` is set on the attached resource to reference back to this VPCAttachment.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpc` | `corev1.ObjectReference` | Yes | Reference to the VPC this attachment belongs to. |
| `interface` | `VPCAttachmentInterface` | Yes | Network interface configuration. |

**VPCAttachmentInterface**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Interface name on the workload (e.g. `galactic0`). Default: `galactic0`. |
| `addresses` | `[]string` | Yes | IPv4 or IPv6 addresses assigned to the interface. Minimum 1. |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `ready` | `bool` | `true` when the attachment is active and the interface is configured. |
| `identifier` | `string` | Platform-assigned unique identifier for this attachment. |

---

## 3. Design Principles

**VPC as an isolation boundary.** Each VPC is an independent L3 domain. Workloads in different VPCs cannot communicate without explicit routing policy.

**Namespace-scoped resources.** Both VPC and VPCAttachment are namespace-scoped, allowing tenant isolation via standard Kubernetes RBAC.

**Interface-centric attachment.** VPCAttachment models the attachment from the workload side — it names the interface and the addresses, rather than describing a port or slot on the VPC side. This keeps attachment creation self-contained in the workload namespace.

---

## 4. Common Verification Commands

```bash
# List all VPCs in a namespace
kubectl get vpcs -n <namespace>

# Check VPC readiness and identifier
kubectl get vpc <name> -n <namespace> -o yaml

# List all attachments in a namespace
kubectl get vpcattachments -n <namespace>

# Check attachment status
kubectl get vpcattachment <name> -n <namespace> \
  -o jsonpath='{.status.ready}'
```
