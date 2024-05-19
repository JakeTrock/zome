GO=go

default: build

get:
	@$(GO) mod tidy

dev:
	@$(GO) run $(filter-out %_test.go,$(wildcard *.go))

test:
	@$(GO) test -v ./...

hbuild:
	@CGO_ENABLED=1 $(GO) build -o zomeSync *.go

runOneNode:
	@go run $(filter-out %_test.go,$(wildcard *.go)) --nodes=localhost:50051

runThreeNodes:
	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --nodes=localhost:50051,localhost:50052,localhost:50053 --logPath="./node1_output.txt"&
	@printf "Started Node 1\n"&

	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --nodes=localhost:50052,localhost:50051,localhost:50053 --logPath="./node2_output.txt"&
	@printf "Started Node 2\n"&

	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --nodes=localhost:50053,localhost:50052,localhost:50051 --logPath="./node3_output.txt"&
	@printf "Started Node 3\nStarted all nodes"

invalidTest:
	@printf "Running invalid test ...\n"
	@go run $(filter-out %_test.go,$(wildcard *.go)) --client=true --batch='./data/invalidTest.txt' --interactive=false
	@sha256sum sqlite-db-localhost-50051.db

clientImportTest:
	@printf "Running SQL data import ...\n"
	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --client=true --batch='./data/clientImport.txt' --interactive=false
	@sha256sum ./data/clientImport.db sqlite-db-localhost-50051.db sqlite-db-localhost-50052.db sqlite-db-localhost-50053.db

wBench:
	@printf "Running SQL data bench write ...\n"
	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --client=true --batch='./data/wHeavy.txt' --interactive=false
	@sha256sum ./data/wHeavy.db sqlite-db-localhost-50051.db sqlite-db-localhost-50052.db sqlite-db-localhost-50053.db

rBench:
	@printf "Running SQL data bench read ...\n"
	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --client=true --batch='./data/rHeavy.txt' --interactive=false
	@sha256sum ./data/wHeavy.db sqlite-db-localhost-50051.db sqlite-db-localhost-50052.db sqlite-db-localhost-50053.db

clientShell:
	@printf "opening client shell ...\n"
	@go run $(filter-out %_test.go,$(wildcard *.go)) --logLevel=0 --client=true --interactive=true

killAll:
	@printf "Stopping all nodes ...\n"
	@pgrep -f "go.* --nodes=localhost:5"
	@pgrep -f "go.* --nodes=localhost:5" | xargs kill
	@printf "Done"

protoCompile:
	@protoc --go_out=./zproto --go_opt=paths=source_relative --proto_path=./zproto --go-grpc_out=./zproto --go-grpc_opt=paths=source_relative ./zproto/raft.proto ./zproto/zreplicate.proto

hashCheck:
	@printf "Checking SHA256 hash of the binary ...\n\nShould be:\n\nclientImport:\n%s\nwBench:\n%s\nrBench:\n%s\n\nIs:\n%s\n%s\n%s\n" "$$(sha256sum ./data/clientImport.db)" "$$(sha256sum ./data/wHeavy.db)" "$$(sha256sum ./data/rHeavy.db)" "$$(sha256sum sqlite-db-localhost-50051.db)" "$$(sha256sum sqlite-db-localhost-50052.db)" "$$(sha256sum sqlite-db-localhost-50053.db)"

clean:
	@rm -f zomeSync
	@rm -f sqlite-*
	@rm -f node*_output.txt