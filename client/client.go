package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	expectedArgFormat = "client put|get host:port filename"
)

type mode string

const (
	modeGet mode = "get"
	modePut mode = "put"
)

type clientState struct {
	mode     mode
	filename string
	host     string
	port     string
}

func init() {
}

func parseArgs(args []string) (clientState, error) {
	state := clientState{}
	if len(args) != 4 {
		return clientState{}, fmt.Errorf("Too few arguments")
	}
	switch mode(strings.ToLower(args[1])) {
	case modeGet:
		state.mode = modeGet
	case modePut:
		state.mode = modePut
	default:
		return clientState{}, fmt.Errorf("Unknown mode")
	}

	host, port, err := net.SplitHostPort(args[2])
	if err != nil {
		return clientState{}, fmt.Errorf("Error parsing host or port: %v", err)
	}
	state.host = host
	state.port = port

	state.filename = args[3]

	return state, nil
}

func main() {
	// client put|get host:port filename
	fmt.Println(os.Args)
}
