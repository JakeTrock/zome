GO=go

default: build

get:
	$(GO) mod tidy

dev:
	$(GO) run $(filter-out %_test.go,$(wildcard *.go))

test:
	$(GO) test -v ./...

hbuild:
	CGO_ENABLED=1 $(GO) build -o zomeD *.go

runSystem:
	go run zomeDbManager --dbPath ./testPath/db.sqlite

clean:
	rm zomeHeadless