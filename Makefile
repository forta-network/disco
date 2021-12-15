.PHONY: run
run:
	@mkdir -p build
	@go build -o build/app
	@./build/app

.PHONY: docker-build
docker-build:
	@docker build -t forta-protocol/disco .

.PHONY: docker-run
docker-run: docker-build
	@docker container rm disco
#	Use host network so we can connect to the IPFS API at localhost:5001
	@docker run --network host --name disco forta-protocol/disco

.PHONY: mocks
mocks:
	@mockgen -source proxy/services/interfaces/interfaces.go -destination proxy/services/interfaces/mocks/mock_interfaces.go

.PHONY: test
test:
	@go test -v -count=1 ./...
