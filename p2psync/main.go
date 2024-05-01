//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"flag"
	"os"

	"github.com/jaketrock/zome/sync/util/raft"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	selectQuery          = "SELECT"
	defaultServerAddress = "localhost:50051"

	clientAttempts          = 5
	clientRetryOnBadCommand = false
)

func main() {
	// server state
	client := flag.Bool("client", false, "Whether to start in client mode.")
	// logging flags
	logLevelInput := flag.Int("logLevel", int(zerolog.InfoLevel), "Logging level. One of panic(5), fatal(4), error(3), warn(2), info(1), debug(0), trace(-1)")
	logPath := flag.String("logPath", "", "Path to optional log output file.")
	// client flags
	serverAddress := flag.String("server", defaultServerAddress, "Address of Raft Cluster Leader.")
	cmdFile := flag.String("batch", "", "Relative path to a command file to run in batch mode.")
	interactive := flag.Bool("interactive", false, "whether batch mode should transition to interactive mode.")
	// server flags
	nodesPtr := flag.String("nodes", "",
		"A comma separated list of node IP:port addresses."+
			" The first node is presumed to be this node and the port number"+
			" is what used to start the local raft server.")
	flag.Parse()

	var logLevel zerolog.Level
	switch *logLevelInput {
	case 0:
		logLevel = zerolog.TraceLevel
	case 1:
		logLevel = zerolog.DebugLevel
	case 2:
		logLevel = zerolog.WarnLevel
	case 3:
		logLevel = zerolog.ErrorLevel
	case 4:
		logLevel = zerolog.FatalLevel
	case 5:
		logLevel = zerolog.PanicLevel
	default:
		logLevel = zerolog.InfoLevel
	}

	if *logPath != "" {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		zerolog.SetGlobalLevel(logLevel)

		// create log file if it doesn't exist
		if _, err := os.Stat(*logPath); os.IsNotExist(err) {
			f, err := os.Create(*logPath)
			if err != nil {
				panic(err)
			}
			log.Logger = log.Output(f)
		} else {
			f, err := os.OpenFile(*logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				panic(err)
			}
			log.Logger = log.Output(f)
		}
	}

	if *client {
		// client mode
		log.Info().Msg("Starting in client mode")

		zClient := InitializeClient(*serverAddress, *cmdFile, *interactive)
		zClient.HandleSignals()
	} else {
		// server mode
		log.Info().Msg("Starting in server mode")

		nodes := ParseNodes(*nodesPtr)
		port := GetLocalPort(nodes)
		otherNodes := GetOtherNodes(nodes)
		localNode := GetLocalNode(nodes)
		log.Info().Msgf(" Starting Raft Server listening at: %v", port)
		log.Info().Msgf("All Node addresses: %v", nodes)
		log.Info().Msgf("Other Node addresses: %v", otherNodes)
		rs := raft.GetInitialServer()
		rs.StartServer(localNode, otherNodes)
	}
}
