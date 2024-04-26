GO=go

default: build

get:
	$(GO) mod tidy

dev:
	$(GO) run devIndRun.go

fe:
	pnpx serve frontend

test:
	$(GO) test -v ./...

hbuild:
	$(GO) build -o zomeHeadless *.go

clean:
	rm zomeHeadless