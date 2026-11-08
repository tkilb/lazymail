.PHONY: all build test clean run

all: build

build:
	@mkdir -p bin
	go build -o bin/lazymail ./cmd/lazymail

test:
	go test -v ./...

run: build
	./bin/lazymail

clean:
	rm -rf bin
