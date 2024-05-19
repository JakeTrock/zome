//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jaketrock/zome/sync/raft"
	"github.com/jaketrock/zome/sync/zproto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const logLevel = zerolog.TraceLevel

var ctx = context.Background()

var bufSize = 1024 * 1024

var nodeListeners = map[string]*bufconn.Listener{
	"localhost:50051": bufconn.Listen(bufSize),
	"localhost:50052": bufconn.Listen(bufSize),
	"localhost:50053": bufconn.Listen(bufSize),
}

func bufDialer(nodeID string) func(context.Context, string) (net.Conn, error) {
	listener, exists := nodeListeners[nodeID]
	fmt.Printf("=========Connecting to Raft server at: %v\n", nodeID)

	if !exists {
		log.Error().Msgf("no listener found for node ID %s", nodeID)
	}
	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}

func mkNode(nodesList string) *raft.Server {
	nodes := ParseNodes(nodesList)
	port := GetLocalPort(nodes)
	otherNodes := GetOtherNodes(nodes)
	localNode := GetLocalNode(nodes)
	log.Info().Msgf("Starting Raft Server listening at: %v", port)
	log.Info().Msgf("All Node addresses: %v", nodes)
	log.Info().Msgf("Other Node addresses: %v", otherNodes)
	rs := raft.GetDefaultServer()

	rs.GetConnection = (func(target string) (*grpc.ClientConn, zproto.RaftClient) {
		// Set up a connection to the server. Note: this is not a blocking call.
		// Connection will be setup in the background.
		conn, err := grpc.DialContext(ctx, "bufnet",
			grpc.WithContextDialer(bufDialer(target)),
			grpc.WithTransportCredentials(insecure.NewCredentials()))

		if err != nil {
			log.Error().Msgf("Failed to connect to server at: %v", target)
			return nil, nil
		} else {
			log.Debug().Msgf("Connected to Raft server at: %v", target)
			c := zproto.NewRaftClient(conn)
			return conn, c
		}
	})

	rs.GetListener = (func(identifier string) (net.Listener, error) {
		lid := "localhost:" + identifier
		listener, exists := nodeListeners[lid]
		if !exists {
			return nil, fmt.Errorf("no listener found for node ID %s", identifier)
		}
		log.Info().Msgf("Starting Raft server ld at: %v", lid)
		return listener, nil
	})

	rs.StartServer(localNode, otherNodes)
	return rs
}

func mkClient(file string) *zomeClient {
	zClient := &zomeClient{
		clientAttempts:          DefaultClient.clientAttempts,
		clientRetryOnBadCommand: DefaultClient.clientRetryOnBadCommand,
		clientServerAddress:     DefaultClient.clientServerAddress,
		clientCmdFile:           file,
		clientInteractive:       true,

		getConnection: func(target string) (*grpc.ClientConn, zproto.RaftClient) {
			// dial leader
			//TODO: add secure dialing
			conn, err := grpc.DialContext(ctx, "bufnet",
				grpc.WithContextDialer(bufDialer(target)),
				grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Error().Msgf("did not connect: %v", err)
			} else {
				log.Info().Msgf("Connected to leader: %v", target)
				rc := zproto.NewRaftClient(conn)
				return conn, rc
			}
			return nil, nil
		},
	}
	zClient.InitializeClient()
	// zClient.HandleSignals()
	return zClient
}

// func randString(length int) string {
// 	bytes := make([]byte, length)
// 	for i := 0; i < length; i++ {
// 		bytes[i] = byte(randInt(65, 90))
// 	}
// 	return string(bytes)
// }

// func randInt(i1, i2 int) int {
// 	return rand.Intn(i2-i1) + i1
// }

type countingWriter struct {
	writer        io.Writer
	filters       []string
	filterResults map[string]int
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.writer.Write(p)
	if err != nil {
		return n, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(p))
	// Scan through each line instead of words
	for scanner.Scan() {
		line := scanner.Text()
		for _, filter := range cw.filters {
			if strings.Contains(line, filter) {
				cw.filterResults[filter]++ // Increment count for the filter
				break                      // Only count once per line for any filter
			}
		}
	}

	return n, scanner.Err()
}

func kill_Test_OneNode(t *testing.T) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(logLevel)
	//mock os.file in memory
	cw := &countingWriter{
		writer:  zerolog.ConsoleWriter{Out: io.Discard}, // can be switched for os.Stdout
		filters: []string{"Command replicated successfully", "Batch executed", "Connected to leader"},
		filterResults: map[string]int{
			"Command replicated successfully": 0,
			"Batch executed":                  0,
			"Connected to leader":             0,
		},
	}
	log.Logger = zerolog.New(cw).With().Timestamp().Logger()

	port1 := 50051
	// servers
	go mkNode(fmt.Sprintf("localhost:%v", port1))
	time.Sleep(4 * time.Second)
	mkClient("./data/clientImport.txt")
	master_hash := getHashOfFile("./data/clientImport.db")

	fh1 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port1))

	// assert log messages are present in correct quantities
	assert.Equal(t, 15638, cw.filterResults["Command replicated successfully"])
	assert.Equal(t, 1, cw.filterResults["Batch executed"])
	assert.Equal(t, 1, cw.filterResults["Connected to leader"])

	// assert all 3 files match master file
	assert.Equal(t, master_hash, fh1)

	//remove all spawned files
	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port1))
	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port1))
}

func Test_ThreeNode(t *testing.T) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(logLevel)
	//mock os.file in memory
	cw := &countingWriter{
		writer:  zerolog.ConsoleWriter{Out: os.Stdout}, // can be switched for os.Stdout
		filters: []string{"Command replicated successfully", "Batch executed", "Connected to leader"},
		filterResults: map[string]int{
			"Command replicated successfully": 0,
			"Batch executed":                  0,
			"Connected to leader":             0,
		},
	}
	log.Logger = zerolog.New(cw).With().Timestamp().Logger()

	port1 := 50051
	port2 := 50052
	port3 := 50053

	// servers
	go mkNode(fmt.Sprintf("localhost:%v,localhost:%v", port1, port2))
	go mkNode(fmt.Sprintf("localhost:%v,localhost:%v", port2, port1))
	// go mkNode(fmt.Sprintf("localhost:%v,localhost:%v,localhost:%v", port1, port2, port3)) //TODO: methinks the problem is that these are parsed wrong and cannot find frens?
	// go mkNode(fmt.Sprintf("localhost:%v,localhost:%v,localhost:%v", port2, port1, port3))
	// go mkNode(fmt.Sprintf("localhost:%v,localhost:%v,localhost:%v", port3, port2, port1))

	time.Sleep(14 * time.Second)
	mkClient("./data/clientImport.txt")

	master_hash := getHashOfFile("./data/clientImport.db")

	fh1 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port1))
	fh2 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port2))
	fh3 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port3))

	// assert log messages are present in correct quantities
	assert.Equal(t, 15638, cw.filterResults["Command replicated successfully"])
	assert.Equal(t, 1, cw.filterResults["Batch executed"])
	assert.Equal(t, 3, cw.filterResults["Connected to leader"])

	// assert all 3 files match master file
	assert.Equal(t, master_hash, fh1)
	assert.Equal(t, master_hash, fh2)
	assert.Equal(t, master_hash, fh3)
}

// func Test_main(t *testing.T) { //TODO: spoof stdout
// 	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
// 	zerolog.SetGlobalLevel(logLevel)
// 	log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
// 	port1 := 50051
// 	port2 := 50052
// 	port3 := 50053

// 	// servers
// 	// var wg sync.WaitGroup
// 	var urls = []string{
// 		fmt.Sprintf(
// 			"localhost:%v,localhost:%v,localhost:%v",
// 			port1, port2, port3),
// 		fmt.Sprintf(
// 			"localhost:%v,localhost:%v,localhost:%v",
// 			port2, port1, port3),
// 		fmt.Sprintf(
// 			"localhost:%v,localhost:%v,localhost:%v",
// 			port3, port2, port1),
// 	}
// 	for _, url := range urls {
// 		// 	// Increment the WaitGroup counter.
// 		// 	wg.Add(1)
// 		// 	// Launch a goroutine to fetch the URL.
// 		// 	go func(url string) {
// 		// 		// Decrement the counter when the goroutine completes.
// 		// 		defer wg.Done()
// 		// 		defer fmt.Println("finished fetching " + url)
// 		go mkNode(url)
// 		// 	}(url)
// 	}
// 	// Wait for all HTTP fetches to complete.
// 	// wg.Wait()

// 	// clients
// 	//wait for servers to start
// 	// for {
// 	// 	if rs1 != nil && rs2 != nil && rs3 != nil {
// 	// 		fmt.Println("finished starting servers")
// 	time.Sleep(14 * time.Second)
// 	mkClient("./data/clientImport.txt")

// 	//test all 3 files match hash of master file
// 	master_hash := getHashOfFile("./data/clientImport.db")

// 	fh1 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port1))
// 	fh2 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port2))
// 	fh3 := getHashOfFile(fmt.Sprintf("./sqlite-db-localhost-%v.db", port3))

// 	//assert all 3 files match master file
// 	assert.Equal(t, master_hash, fh1)
// 	assert.Equal(t, master_hash, fh2)
// 	assert.Equal(t, master_hash, fh3)

// 	// clean up after

// 	// remove all dbs
// 	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port1))
// 	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port2))
// 	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port3))
// 	//remove all raftlogs
// 	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port1))
// 	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port2))
// 	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port3))
// 	// break
// 	// 	}
// 	// }
// }

func getHashOfFile(path string) string {
	hash := sha256.New()
	f, err := os.Open(path)
	if err != nil {
		log.Error().Msgf("Error opening file: %v", err)
		return ""
	}
	defer f.Close()
	if _, err := io.Copy(hash, f); err != nil {
		log.Error().Msgf("Error copying file to hash: %v", err)
		return ""
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
