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

	if host == "" {
		return clientState{}, fmt.Errorf("Host can't be empty")
	}

	state.filename = args[3]

	return state, nil
}

func handleState(s clientState) {
	switch s.mode {
	case modePut:
		fmt.Println("Doing put")
	case modeGet:
		fmt.Println("Doing get")
	}
}

func main() {
	state, err := parseArgs(os.Args)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Expected", expectedArgFormat)
		return
	}
	handleState(state)
}
