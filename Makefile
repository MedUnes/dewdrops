
build:
	CGO_ENABLED=0 go build -ldflags="-extldflags=-static" -o dewdrops
format:
	go fmt ./...
format-check:
	@UNFORMATTED_FILES=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED_FILES" ]; then \
		echo "::error::The following files are not formatted correctly:"; \
		echo "$$UNFORMATTED_FILES"; \
		echo "--- Diff ---"; \
		gofmt -d .; \
		exit 1; \
	fi
run:
	go run main.go
install:
	$(MAKE) build
	sudo mv ./dewdrops /usr/local/bin
test:
	go run gotest.tools/gotestsum@latest --format=testdox -- -covermode=atomic -coverprofile=coverage.txt ./...
