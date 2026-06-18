kubectl delete crd nodegroups.config-manager.yuno.org
kubectl delete crd nodeinstances.config-manager.yuno.org
make manifests
make install
make deploy IMG=192.168.0.80:15000/node-group:v1.0