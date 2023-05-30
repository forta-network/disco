.PHONY: run
build:
	@mkdir -p build
	@go build -o build/app

.PHONY: build
run: build
	@REGISTRY_CONFIGURATION_PATH=dev-config.yaml ./build/app

.PHONY: docker-build
docker-build:
	@docker build -t forta-network/disco .

.PHONY: docker-run
docker-run: docker-build
	@docker container rm disco
#	Use host network so we can connect to the IPFS API at localhost:5001
	@docker run --network host --name disco forta-network/disco

.PHONY: mocks
mocks:
	@mockgen -source proxy/services/interfaces/interfaces.go -destination proxy/services/interfaces/mocks/mock_interfaces.go

.PHONY: test
test:
	@go test -v -count=1 ./...

.PHONY: e2e
e2e:
	docker build -t localhost:1970/test -f e2e/Dockerfile.empty ./e2e
	cd e2e && E2E_TEST=1 go test -v .
