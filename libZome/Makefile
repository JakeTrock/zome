GO=go

default: build

get:
	$(GO) mod tidy

dev:
	$(GO) run $(filter-out %_test.go,$(wildcard *.go))

fe:
	pnpx serve frontend

test:
	$(GO) test -v ./...

hbuild:
	$(GO) build -o zomeHeadless *.go

clean:
	rm zomeHeadless