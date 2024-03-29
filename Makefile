.PHONY: build
build:
	@mkdir -p build
	@go build -o build/disco

.PHONY: run
run: build
	@REGISTRY_CONFIGURATION_PATH=dev-config.yaml ./build/disco

.PHONY: docker-build
docker-build:
	@docker build -t forta-network/disco .

.PHONY: docker-run
docker-run: docker-build
    # use host network so we can connect to the IPFS API at localhost:5001
	@docker run --rm --network host --name disco forta-network/disco

.PHONY: mocks
mocks:
	@mockgen -source interfaces/interfaces.go -destination interfaces/mocks/mock_interfaces.go
	@mockgen -source drivers/multidriver/multidriver.go -destination drivers/multidriver/mocks/mock_multidriver.go

.PHONY: test
test:
	@go test -v -count=1 -covermode=count -coverprofile=coverage.out ./...

.PHONY: cover
cover: test
	go tool cover -func=coverage.out -o=coverage.out

.PHONY: coverage
coverage: test
	go tool cover -func=coverage.out | grep total | awk '{print substr($$3, 1, length($$3)-1)}'

.PHONY: e2e
e2e: build
	docker pull nats:2.4
	docker tag nats:2.4 localhost:1970/test
	cd e2e && E2E_TEST=1 go test -v .
