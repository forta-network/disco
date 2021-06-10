.PHONY: run
run:
	@mkdir -p build
	@go build -o build/app
	@./build/app

.PHONY: docker-build
docker-build:
	@docker build -t openzeppelin/disco .

.PHONY: docker-run
docker-run: docker-build
#	Use host network so we can connect to the IPFS API at localhost:5001
	@docker run -d --network host openzeppelin/disco
