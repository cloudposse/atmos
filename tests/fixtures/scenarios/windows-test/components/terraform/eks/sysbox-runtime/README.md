# Component: `eks/sysbox-runtime`

This component deploys Sysbox for EKS using the official `sysbox-deploy-k8s` DaemonSet approach, enabling secure nested container execution (Docker-in-Docker, Kubernetes-in-Docker) with VM-like isolation.

## Architecture Overview

```
+-----------------------------------------------------------------------------+
|                           EKS Cluster (v1.33)                               |
+-----------------------------------------------------------------------------+
|                                                                             |
|  +---------------------+     +------------------------------------------+   |
|  |   Default NodePool  |     |        Sysbox NodePool                   |   |
|  |   (AL2023/containerd)|     |   (Ubuntu 22.04 + CRI-O + Sysbox)       |   |
|  |                     |     |                                          |   |
|  |  Regular workloads  |     |  DaemonSet installs:                     |   |
|  |                     |     |  - CRI-O (replaces containerd)           |   |
|  |                     |     |  - Sysbox CE runtime                     |   |
|  |                     |     |                                          |   |
|  |                     |     |  Sandboxed workloads with                |   |
|  |                     |     |  runtimeClassName: sysbox-runc           |   |
|  +---------------------+     +------------------------------------------+   |
|                                                                             |
|  +-----------------------------------------------------------------------+  |
|  |                     RuntimeClass: sysbox-runc                         |  |
|  |   handler: sysbox-runc                                                |  |
|  |   nodeSelector: sysbox-install=yes                                    |  |
|  |   tolerations: sandbox-runtime=sysbox:NoSchedule                      |  |
|  +-----------------------------------------------------------------------+  |
|                                                                             |
|  +-----------------------------------------------------------------------+  |
|  |                 DaemonSet: sysbox-deploy (kube-system)                |  |
|  |   Targets: nodes with sysbox-install=yes                              |  |
|  |   Installs: CRI-O + Sysbox, restarts kubelet                          |  |
|  +-----------------------------------------------------------------------+  |
|                                                                             |
+-----------------------------------------------------------------------------+
```

## How It Works

### DaemonSet-Based Installation (Official Approach)

Following the [official Nestybox Kubernetes installation guide](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md):

1. **Node Provisioning**: Karpenter provisions Ubuntu 22.04 nodes with `sysbox-install=yes` label
2. **Node Joins Cluster**: Node joins running containerd (default EKS runtime)
3. **DaemonSet Triggers**: `sysbox-deploy-k8s` pod starts on the node
4. **CRI-O Installation**: DaemonSet installs CRI-O and configures it as the runtime
5. **Sysbox Installation**: DaemonSet installs Sysbox and configures sysbox-runc handler
6. **Kubelet Restart**: Kubelet restarts to use CRI-O (~2-3 min total)
7. **Node Ready**: Node is ready for Sysbox workloads

### Why CRI-O?

Sysbox **requires CRI-O** (not containerd) because:
- CRI-O creates **user namespace BEFORE network namespace**
- containerd creates network namespace first
- Sysbox needs user namespace first to mount sysfs without "operation not permitted" errors

This is a fundamental Linux kernel requirement, not a configuration issue.

## Deployment Order

1. **Deploy EKS cluster** (`eks/cluster`)
2. **Deploy Karpenter** (`eks/karpenter`)
3. **Deploy sysbox-runtime** (this component) - Creates RuntimeClass, RBAC, DaemonSet
4. **Deploy sysbox-node-pool** (`eks/sysbox-node-pool`) - Creates NodePool
5. **Deploy workload** - Triggers node provisioning

## Usage

### Deploy a pod with Sysbox

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-sandbox-pod
spec:
  runtimeClassName: sysbox-runc
  containers:
  - name: my-container
    image: my-image
```

### Run Docker inside a container

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: docker-in-docker
spec:
  runtimeClassName: sysbox-runc
  containers:
  - name: dind
    image: docker:dind
    # No privileged flag needed with Sysbox!
```

## Troubleshooting

### Check DaemonSet status
```bash
kubectl get daemonset -n kube-system sysbox-deploy
kubectl logs -n kube-system -l name=sysbox-deploy -f
```

### Check node status
```bash
kubectl get nodes -l sysbox-install=yes -o wide
```

### Check RuntimeClass
```bash
kubectl get runtimeclass sysbox-runc -o yaml
```

### Check if CRI-O is running on node
```bash
# From debug pod or SSM session:
systemctl status crio
crictl info
```

## Node Requirements

Per [Nestybox documentation](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md):

- **OS**: Ubuntu Jammy (22.04), Focal (20.04), or Noble (24.04)
- **Kernel**: 5.4+
- **Resources**: Minimum 4 vCPUs, 4GB RAM per node
- **Kubernetes**: v1.29 - v1.33

## References

- [Sysbox K8s Installation Guide](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md)
- [Sysbox EKS Guide](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s-distros.md)
- [Sysbox GitHub](https://github.com/nestybox/sysbox)
- [Ubuntu EKS AMIs](https://cloud-images.ubuntu.com/docs/aws/eks/)
- [Kubernetes RuntimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/)
