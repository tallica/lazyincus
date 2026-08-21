.PHONY: run build test vet lint clean

run:
	go run .

build:
	go build -o lazyincus .

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -f lazyincus
