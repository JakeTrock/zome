GO=go

default: build

get:
	$(GO) mod tidy

dev:
	$(GO) run $(filter-out %_test.go,$(wildcard *.go))

test:
	$(GO) test -v ./...

hbuild:
	CGO_ENABLED=1 $(GO) build -o zomeSync *.go

runOneNode:
	go run $(filter-out %_test.go,$(wildcard *.go)) -nodes=localhost:50051

runThreeNodes:
	go run $(filter-out %_test.go,$(wildcard *.go)) -nodes=localhost:50051,localhost:50052,localhost:50053 >/tmp/node3_output.txt &
	echo "Started Node 3"

	go run $(filter-out %_test.go,$(wildcard *.go)) -nodes=localhost:50052,localhost:50051,localhost:50053 >/tmp/node2_output.txt &
	echo "Started Node 2"

	go run $(filter-out %_test.go,$(wildcard *.go)) -nodes=localhost:50053,localhost:50052,localhost:50051 >/tmp/node1_output.txt &
	echo "Started Node 1"

clientImportTest:
	echo "Running SQL data import ..."
	go run $(filter-out %_test.go,$(wildcard *.go)) -client=true -batch='./data/clientImport.txt' --interactive=false
	sha256sum ./data/clientImport.db sqlite-db-localhost-50051.db

wBench:
	echo "Running SQL data bench write ..."
	go run $(filter-out %_test.go,$(wildcard *.go)) -client=true -batch='./data/wHeavy.txt' --interactive=false
	sha256sum ./data/wHeavy.db sqlite-db-localhost-50051.db

rBench:
	echo "Running SQL data bench read ..."
	go run $(filter-out %_test.go,$(wildcard *.go)) -client=true -batch='./data/rHeavy.txt' --interactive=false
	sha256sum ./data/wHeavy.db sqlite-db-localhost-50051.db

clientShell:
	echo "opening client shell ..."
	go run $(filter-out %_test.go,$(wildcard *.go)) -client=true --interactive=true

killAll:
	echo "Stopping all nodes ..."
	pgrep -f "go.* -nodes=localhost:5"
	pgrep -f "go.* -nodes=localhost:5" | xargs kill
	echo "Done"

protoCompile:
	protoc --go_out=util/proto --go_opt=paths=source_relative --proto_path=util/proto --go-grpc_out=util/proto --go-grpc_opt=paths=source_relative ./util/proto/raft.proto ./util/proto/zreplicate.proto

hashCheck:
	echo "Checking SHA256 hash of the binary ..."
	echo "Should be: a60c6591c09748c0eec2b3ff16b0e2cd5065e0feac8ff0811fc8b90224e881b7"
	sha256sum sqlite-db-localhost-50051.db
	sha256sum sqlite-db-localhost-50052.db
	sha256sum sqlite-db-localhost-50052.db

clean:
	rm -f zomeSync
	rm -f sqlite-*