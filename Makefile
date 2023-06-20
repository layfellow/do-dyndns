BIN = do-dyndns

build:
	go build -o build/$(BIN)

dependencies:
	go get -u
	go mod tidy

install:
	go env -w GOBIN=$$HOME/bin
	go install

clean:
	rm -rf build

.PHONY: build dependencies install clean
