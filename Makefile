.PHONY: dockerbuild-example
dockerbuild-example:
	@eval $(minikube docker-env)
	@docker build . --build-arg module=example -t systest:example

.PHONY: localbuild-example
localbuild-example:
	@go test -c ./example/ -o build/example.test
	@docker build . -f Localfile --build-arg module=example -t systest:example

.PHONY: run-example
run-example:
	@kubectl run -i --tty --image systest:example systest --restart=Never --image-pull-policy=IfNotPresent --rm -- example -test.v