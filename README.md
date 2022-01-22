Systest
---

1. Install minikube

https://minikube.sigs.k8s.io/docs/start/

1. Grant permissions for default/default serviceaccount so that it will be allowed to create namespaces inside the cluster.

```bash
kubectl create clusterrolebinding serviceaccounts-cluster-admin \
  --clusterrole=cluster-admin --group=system:serviceaccounts
```

3. Build test image for `example` module, either with `make dockerbuild-example` or `make localbuild-example`.

4. `run-example`

The command will run one-shot container inside the cluster with the test in `example` module. Namespace will be prefixed with the `test` keyword.
The test will setup cluster, setup partition between some of the nodes in the cluster and heal partition after 30 minutes.

## logging

Follow instructions https://grafana.com/docs/loki/latest/installation/helm/.

## chaos-mesh

https://chaos-mesh.org/docs/quick-start/

```bash
curl -sSL https://mirrors.chaos-mesh.org/v2.1.2/install.sh | bash
```
