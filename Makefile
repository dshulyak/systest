test_name ?= TestSmeshing
version_info ?= $(shell git rev-parse --short HEAD)
image_name ?= yashulyak/systest:$(version_info)
smesher_image ?= spacemeshos/go-spacemesh-dev:fastnet
test_pod_name ?= systest-$(version_info)
parallel ?= 1
size ?= 10

.PHONY: docker
docker:
	@DOCKER_BUILDKIT=1 docker build . -t $(image_name)

.PHONY: push
push:
	docker push $(image_name)

.PHONY: run
run: launch watch

.PHONY: launch
launch:
	@kubectl run --image $(image_name) $(test_pod_name) \
	--restart=Never \
	--image-pull-policy=IfNotPresent -- \
	tests -test.v -test.timeout=0 -test.run=$(test_name) -test.parallel=$(parallel) -size=$(size) -image=$(smesher_image) -level=debug

.PHONY: watch
watch:
	@kubectl wait --for=condition=ready pod/$(test_pod_name)
	@kubectl logs $(test_pod_name) -f

.PHONY: clean
clean:
	@kubectl delete pod/$(test_pod_name)