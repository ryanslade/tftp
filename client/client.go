package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/ryanslade/tftp/common"
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

// handle reading a local file and sending it to the server
func handlePut(filename, host, port string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("Error opening file: %v", err)
	}
	defer f.Close()

	br := bufio.NewReader(f)

	// Create conn and remoteAddr
	remoteAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		return fmt.Errorf("Error resolving address: %v", err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	})
	if err != nil {
		return fmt.Errorf("Error setting up connection: %v", err)
	}

	// Send WRQ packet
	wrq := common.RequestPacket{
		OpCode:   common.OpWRQ,
		Filename: filename,
		Mode:     "octet",
	}

	_, err = conn.WriteTo(wrq.ToBytes(), remoteAddr)
	if err != nil {
		return fmt.Errorf("Error sending WRQ packet: %v", err)
	}

	// Get the ACK
	ackBuf := make([]byte, 4)
	_, addr, err := conn.ReadFrom(ackBuf)
	if err != nil {
		return fmt.Errorf("Error reading ACK packet: %v", err)
	}
	_, err = common.ParseAckPacket(ackBuf)
	if err != nil {
		return fmt.Errorf("Error parsing ACK packet: %v", err)
	}

	// ReadLoop
	// TODO: Rename. ReadLoop is confusing.. read from what? File or Connection?
	common.ReadFileLoop(br, conn, addr, common.BlockSize)

	return nil
}

func handleState(s clientState) {
	switch s.mode {
	case modePut:
		if err := handlePut(s.filename, s.host, s.port); err != nil {
			log.Printf("Error performing put: %v", err)
		}

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
