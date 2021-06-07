.PHONY: run
run:
	@mkdir -p build
	@go build -o build/app
	@./build/app
