Systest
---

1. Install minikubd

2. Grant permissions for default/default serviceaccount to create namespaces.

```bash
kubectl create clusterrolebinding serviceaccounts-cluster-admin \
  --clusterrole=cluster-admin --group=system:serviceaccounts
```

3. Build test image for example module, either with `make dockerbuild-example` or `make localbuild-example`.

4. `run-example`

Will run one-shot container inside the cluster. Namespace will be prefixed with `test` keyword.