GO=go

default: build

get:
	$(GO) mod tidy

hbuild:
	$(GO) build -o zomeHeadless headless/headless.go

clean:
	rm zomeHeadless

makeall:
	rm -rf headlessBin/*

	mkdir -p headlessBin/windows
	GOOS=windows GOARCH=amd64 go build -o headlessBin/windows/win-amd64.exe headless/headless.go
	GOOS=windows GOARCH=arm64 go build -o headlessBin/windows/win-arm64.exe headless/headless.go
	GOOS=windows GOARCH=386 go build -o headlessBin/windows/win-386.exe headless/headless.go
	GOOS=windows GOARCH=arm go build -o headlessBin/windows/win-arm.exe headless/headless.go

	mkdir -p headlessBin/linux
	GOOS=linux GOARCH=amd64 go build -o headlessBin/linux/lin-amd64 headless/headless.go
	GOOS=linux GOARCH=arm64 go build -o headlessBin/linux/lin-arm64 headless/headless.go
	GOOS=linux GOARCH=386 go build -o headlessBin/linux/lin-386 headless/headless.go
	GOOS=linux GOARCH=arm go build -o headlessBin/linux/lin-arm headless/headless.go
	GOOS=linux GOARCH=mips go build -o headlessBin/linux/lin-mips headless/headless.go
	GOOS=linux GOARCH=mips64 go build -o headlessBin/linux/lin-mips64 headless/headless.go
	GOOS=linux GOARCH=mips64le go build -o headlessBin/linux/lin-mips64le headless/headless.go
	GOOS=linux GOARCH=mipsle go build -o headlessBin/linux/lin-mipsle headless/headless.go
	GOOS=linux GOARCH=ppc64 go build -o headlessBin/linux/lin-ppc64 headless/headless.go
	GOOS=linux GOARCH=ppc64le go build -o headlessBin/linux/lin-ppc64le headless/headless.go
	GOOS=linux GOARCH=riscv64 go build -o headlessBin/linux/lin-riscv64 headless/headless.go
	GOOS=linux GOARCH=s390x go build -o headlessBin/linux/lin-s390x headless/headless.go

	mkdir -p headlessBin/mac
	GOOS=darwin GOARCH=amd64 go build -o headlessBin/mac/mac-amd64 headless/headless.go
	GOOS=darwin GOARCH=arm64 go build -o headlessBin/mac/mac-arm64 headless/headless.go

	mkdir -p headlessBin/bsd
	GOOS=freebsd GOARCH=amd64 go build -o headlessBin/bsd/fbsd-amd64 headless/headless.go
	GOOS=freebsd GOARCH=arm64 go build -o headlessBin/bsd/fbsd-arm64 headless/headless.go
	GOOS=openbsd GOARCH=amd64 go build -o headlessBin/bsd/obsd-amd64 headless/headless.go
	GOOS=openbsd GOARCH=arm64 go build -o headlessBin/bsd/obsd-arm64 headless/headless.go
	GOOS=netbsd GOARCH=amd64 go build -o headlessBin/bsd/nbsd-amd64 headless/headless.go
	GOOS=netbsd GOARCH=arm64 go build -o headlessBin/bsd/nbsd-arm64 headless/headless.go
	GOOS=freebsd GOARCH=386 go build -o headlessBin/bsd/fbsd-386 headless/headless.go
	GOOS=openbsd GOARCH=386 go build -o headlessBin/bsd/obsd-386 headless/headless.go
	GOOS=netbsd GOARCH=386 go build -o headlessBin/bsd/nbsd-386 headless/headless.go
	GOOS=freebsd GOARCH=arm go build -o headlessBin/bsd/fbsd-arm headless/headless.go
	GOOS=openbsd GOARCH=arm go build -o headlessBin/bsd/obsd-arm headless/headless.go
	GOOS=netbsd GOARCH=arm go build -o headlessBin/bsd/nbsd-arm headless/headless.go
	GOOS=freebsd GOARCH=riscv64 go build -o headlessBin/bsd/fbsd-riscv64 headless/headless.go
