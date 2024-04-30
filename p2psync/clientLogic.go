package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/jaketrock/zome/sync/util"

	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	proto "github.com/jaketrock/zome/sync/util/proto"
)

// Parse command to determine whether it is RO or making changes
func CommandIsRO(query string) bool {
	// input trimmed at client
	queryUpper := strings.ToUpper(query)
	return strings.HasPrefix(queryUpper, selectQuery)
}

func Connect(addr string) {

	// dial leader
	var err error
	conn, err = grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
		os.Exit(1)
	}

	raftServer = proto.NewRaftClient(conn)
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

func Format(commandString string, sanitize bool) ([]string, error) {
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
		m, _ = commandSplit.FindNextMatch(m)
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

func Execute(commands []string) (string, error) {
	var buf bytes.Buffer
	numCommands := len(commands)
	fmt.Printf("Have num SQL commands to process: %v\n", numCommands)
	for i, command := range commands {
		if i%1000 == 0 {
			fmt.Printf("Processing command %v of %v\n", i+1, numCommands)
		}
		commandRequest := proto.ClientCommandRequest{}
		if CommandIsRO(command) {
			commandRequest.Query = command
		} else {
			commandRequest.Command = command
		}

		var result *proto.ClientCommandResponse
		var err error

		// 5 reconn attempts if leader failure
		attempts := 5
		for i := 1; i <= attempts; i++ {

			result, err = raftServer.ClientCommand(context.Background(), &commandRequest)
			if result == nil {
				fmt.Println("Trying again")
			}
			if err != nil {
				util.Log(util.ERROR, "Error sending command to node %v err: %v", raftServer, err)
				return "", err
			}

			if result.ResponseStatus == uint32(codes.FailedPrecondition) {
				util.Log(util.WARN, "Reconnecting with new leader: %v (%v/%v)", result.NewLeaderId, i+1, attempts)
				Connect(result.NewLeaderId)
				continue
			}

			// TODO: may want to downgrade log fatals and not just abort if any query fails everywhere in the code
			if result.ResponseStatus != uint32(codes.OK) {
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

func Repl() {
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
		text, _ := reader.ReadString('\n')
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
			text, _ = reader.ReadString('\n')
			text = strings.TrimSpace(text)

			// imitating sqlite3 behavior
			if text == "" {
				// EOF received (^D)
				exit = true
				break
			}
		}
		buf.WriteString(text)
		commands, err := Format(buf.String(), true)
		if err != nil {
			fmt.Println(err)
		}
		output, err := Execute(commands)
		if err != nil {
			fmt.Println(err)
		} else if output != "" {
			fmt.Print(output)
		}
		buf.Reset()
	}

	conn.Close()
}

func Batch(batchedCommands string) {
	commands, _ := Format(batchedCommands, false)
	_, err := Execute(commands)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Success")
	}
}
