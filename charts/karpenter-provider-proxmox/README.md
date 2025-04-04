# karpenter-provider-yandex

![Version: 0.0.1](https://img.shields.io/badge/Version-0.0.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.1.0](https://img.shields.io/badge/AppVersion-v0.1.0-informational?style=flat-square)

Karpenter for Yandex Cloud.

**Homepage:** <https://github.com/tufitko/karpenter-provider-yandex>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| tufitko |  | <https://github.com/tufitko> |

## Source Code

* <https://github.com/tufitko/karpenter-provider-yandex>

## Yandex permissions

```shell
# Create role Karpenter
pveum role add karpenter -privs "VM.Audit VM.Config.Disk Datastore.Allocate Datastore.AllocateSpace Datastore.Audit"

# Create user and grant permissions
pveum user add kubernetes-karpenter@pve
pveum aclmod / -user kubernetes-karpenter@pve -role karpenter
pveum user token add kubernetes-karpenter@pve karpenter -privsep 0
```

## Helm values example

```yaml
# karpenter-provider-yandex.yaml

config:
  clusters:
    - url: https://cluster-api-1.exmple.com:8006/api2/json
      insecure: false
      token_id: "kubernetes-csi@pve!csi"
      token_secret: "key"
      region: cluster-1

# Deploy controller only on control-plane nodes
nodeSelector:
  node-role.kubernetes.io/control-plane: ""
tolerations:
  - key: node-role.kubernetes.io/control-plane
    effect: NoSchedule
```

## Deploy

```shell
# Install Karpenter
helm upgrade -i --namespace=kube-system -f karpenter-provider-yandex.yaml \
  karpenter-provider-yandex oci://ghcr.io/tufitko/charts/karpenter-provider-yandex
```

