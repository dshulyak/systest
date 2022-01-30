test_pod_name ?= systest
test_name ?= TestExample

.PHONY: docker
docker:
	@eval $(minikube docker-env)
	@DOCKER_BUILDKIT=1 docker build . -t systest:example

.PHONY: run
run: launch watch

.PHONY: launch
launch:
	@kubectl run --image systest:example $(test_pod_name) \
	--restart=Never \
	--image-pull-policy=IfNotPresent -- \
	tests -test.v -test.timeout=0 -test.run=$(test_name)

.PHONY: watch
watch:
	@kubectl wait --for=condition=ready pod/$(test_pod_name)
	@kubectl logs $(test_pod_name) -f

.PHONY: clean
clean:
	@kubectl delete pod/$(test_pod_name)