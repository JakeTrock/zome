package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/dlclark/regexp2"

	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaketrock/zome/sync/zproto"
)

type zomeClient struct {
	raftServer zproto.RaftClient
	conn       *grpc.ClientConn

	clientAttempts          int
	clientRetryOnBadCommand bool
	clientInteractive       bool
	clientCmdFile           string
	clientServerAddress     string

	getConnection func(target string) (*grpc.ClientConn, zproto.RaftClient)
}

var DefaultClient = &zomeClient{

	clientAttempts:          5,
	clientRetryOnBadCommand: false,
	clientServerAddress:     "localhost:50051",
	clientCmdFile:           "",
	clientInteractive:       true,

	getConnection: func(target string) (*grpc.ClientConn, zproto.RaftClient) {
		// dial leader
		//TODO: add secure dialing, switch to using grpc.WithContextDialer()
		conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Error().Msgf("Failed to connect: %v", err)
			return nil, nil
		} else {
			log.Info().Msgf("Connected to leader: %v", target)
			rc := zproto.NewRaftClient(conn)
			return conn, rc
		}
	},
}

func (zc *zomeClient) InitializeClient() {
	for i := 0; i < zc.clientAttempts; i++ {
		zc.Connect(zc.clientServerAddress)
		if zc.raftServer != nil {
			break
		} else if i == zc.clientAttempts-1 {
			log.Error().Msgf("Failed to connect to leader after %v attempts", zc.clientAttempts)
			os.Exit(1)
		}
		// logarithmic delay
		delTime := time.Duration(math.Pow(2, float64(i+1))) * time.Second
		log.Warn().Msgf("Delaying leader connection attempt %v of %v for %v", i+1, zc.clientAttempts, delTime)
		time.Sleep(delTime)
	}
	if zc.clientCmdFile != "" {
		content, err := os.ReadFile(zc.clientCmdFile)
		if err != nil {
			log.Error().Msgf("Error reading file: %v", err)
			os.Exit(1)
		}

		zc.Batch(string(content))

		if !zc.clientInteractive {
			os.Exit(0)
		}
	}
}

// Parse command to determine whether it is RO or making changes
func CommandIsRO(query string) bool {
	// input trimmed at client
	queryUpper := strings.ToUpper(query)
	return strings.HasPrefix(queryUpper, selectQuery)
}

func (zc *zomeClient) Connect(addr string) {
	zc.conn, zc.raftServer = zc.getConnection(addr)
}

// ====

func Sanitize(command string) error {
	var err error

	command = strings.Replace(command, "\n", "", -1)

	if len(command) > 1 && command[:1] == "." {
		return errors.New("sqlite3 .* syntax not supported")
	}

	// lazy case-insensitive but wtv for now
	upperC := strings.ToUpper(command)

	if strings.Contains(upperC, "RANDOM()") {
		return errors.New("random() or other non-determinstic commands not supported")
	}

	// TODO: deal with utc modifier
	if strings.Contains(upperC, "('NOW'") || strings.Contains(upperC, "'NOW')") {
		return errors.New("now or other non-determinstic commands not supported")
	}

	if strings.Contains(upperC, "BEGIN TRANSACTION") ||
		strings.Contains(upperC, "COMMIT TRANSACTION") ||
		strings.Contains(upperC, "END TRANSACTION") ||
		strings.Contains(upperC, "ROLLBACK TRANSACTION") {

		return errors.New("transactions not supported")
	}
	// doesn't deal with user-defined functions as enabled in
	// SQLite C interface.

	return err
}

func (zc *zomeClient) Format(commandString string, sanitize bool) ([]string, error) {
	// Split commands using regex to handle queries with ; in them
	commandSplit := regexp2.MustCompile(`(?:^|;)([^';]*(?:'[^']*'[^';]*)*)(?=;|$)`, 0)

	var commands []string
	m, err := commandSplit.FindStringMatch(commandString)

	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("invalid command")
	}

	for m != nil {
		commands = append(commands, m.String())
		m, err = commandSplit.FindNextMatch(m)
		if err != nil {
			log.Error().Msgf("Error parsing command: %v", err)
		}
	}

	if sanitize {
		for _, comm := range commands {
			err := Sanitize(comm)
			if err != nil {
				return make([]string, 0), err
			}
		}
	}

	return commands, nil
}

func (zc *zomeClient) Execute(commands []string) (string, error) {
	var buf bytes.Buffer
	numCommands := len(commands)
	fmt.Printf("Have %v SQL commands to process\n", numCommands)
	for i, command := range commands {
		if i%1000 == 0 {
			fmt.Printf("Processing command %v of %v\n", i+1, numCommands)
		}
		commandRequest := zproto.ClientCommandRequest{}
		if CommandIsRO(command) {
			commandRequest.Query = command
		} else {
			commandRequest.Command = command
		}

		var result *zproto.ClientCommandResponse
		var err error

		// x reconn attempts if leader failure
		for i := 0; i < zc.clientAttempts; i++ {
			result, err = zc.raftServer.ClientCommand(context.Background(), &commandRequest)
			if result == nil {
				log.Error().Msgf("Error sending command to node %v err: %v", zc.raftServer, err)
			}
			if err != nil {
				log.Error().Msgf("Error sending command to node %v err: %v", zc.raftServer, err)
				if zc.clientRetryOnBadCommand {
					continue
				}
				return "", err
			}

			if result.ResponseStatus == uint32(codes.FailedPrecondition) {
				log.Warn().Msgf("Reconnecting with new leader: %v (%v/%v)", result.NewLeaderId, i+1, zc.clientAttempts)
				zc.Connect(result.NewLeaderId)
				continue
			}

			// TODO: downgrade log fatals and not just abort if any query fails everywhere in the code
			if result.ResponseStatus != uint32(codes.OK) {
				if zc.clientRetryOnBadCommand {
					continue
				}
				return "", errors.New(result.QueryResponse)
			} else {
				break
			}

		}

		if CommandIsRO(command) {
			buf.WriteString(result.QueryResponse)
		}
	}

	return buf.String(), nil
}

func (zc *zomeClient) HandleSignals() {
	//handle signals appropriately; not quite like sqlite3
	c := make(chan os.Signal, 1)
	signal.Notify(c)
	go func() {
		for sig := range c {
			if sig == os.Interrupt {
				os.Exit(0)
			}
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	var buf bytes.Buffer
	exit := false

	for !exit {
		fmt.Print("ZDB> ")
		text, err := reader.ReadString('\n')
		if err != nil {
			log.Error().Msgf("Error reading input: %v", err)
		}
		text = strings.TrimSpace(text)

		if text == "" || text == "exit" {
			// EOF received (^D)
			os.Exit(0)
		}

		if text == "\n" {
			continue
		}

		for text == "" || text[len(text)-1:] != ";" {
			buf.WriteString(text)
			fmt.Print("     ...> ")
			text, err := reader.ReadString('\n')
			if err != nil {
				log.Error().Msgf("Error reading input: %v", err)
			}
			text = strings.TrimSpace(text)

			// imitating sqlite3 behavior
			if text == "" {
				// EOF received (^D)
				exit = true
				break
			}
		}
		buf.WriteString(text)
		commands, err := zc.Format(buf.String(), true)
		if err != nil {
			log.Error().Msgf("Error parsing command: %v", err)
		}
		output, err := zc.Execute(commands)
		if err != nil {
			log.Error().Msgf("Error executing command: %v", err)
		} else if output != "" {
			log.Info().Msgf("Output: %v", output)
		}
		buf.Reset()
	}

	zc.conn.Close()
}

func (zc *zomeClient) Batch(batchedCommands string) {
	commands, err := zc.Format(batchedCommands, false)
	if err != nil {
		log.Error().Msgf("Error parsing command: %v", err)
	}
	_, err = zc.Execute(commands)
	if err != nil {
		log.Error().Msgf("Error executing command: %v", err)
	} else {
		log.Info().Msgf("Batch executed")
	}
}
