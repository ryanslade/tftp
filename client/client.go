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
	address  string
}

// TODO: Maybe default to port 69?
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
	if host == "" {
		return clientState{}, fmt.Errorf("Host can't be blank")
	}
	if port == "" {
		return clientState{}, fmt.Errorf("Port can't be blank")
	}
	state.address = args[2]
	state.filename = args[3]

	return state, nil
}

func getAddrAndConn(address string) (net.Addr, net.PacketConn, error) {
	// Create conn and remoteAddr
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("Error resolving address: %v", err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("Error setting up connection: %v", err)
	}

	return serverAddr, conn, nil
}

// handle reading a local file and sending it to the server
func handlePut(filename, address string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("Error opening file: %v", err)
	}
	defer f.Close()

	br := bufio.NewReader(f)

	serverAddr, conn, err := getAddrAndConn(address)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send WRQ packet
	wrq := common.RequestPacket{
		OpCode:   common.OpWRQ,
		Filename: filename,
		Mode:     "octet",
	}

	_, err = conn.WriteTo(wrq.ToBytes(), serverAddr)
	if err != nil {
		return fmt.Errorf("Error sending WRQ packet: %v", err)
	}

	// Get the ACK
	ackBuf := make([]byte, 4)
	_, remoteAddr, err := conn.ReadFrom(ackBuf)
	if err != nil {
		return fmt.Errorf("Error reading ACK packet: %v", err)
	}
	_, err = common.ParseAckPacket(ackBuf)
	if err != nil {
		return fmt.Errorf("Error parsing ACK packet: %v", err)
	}

	common.ReadFileLoop(br, conn, remoteAddr, common.BlockSize)

	return nil
}

func handleGet(filename string, address string) error {
	serverAddr, conn, err := getAddrAndConn(address)
	if err != nil {
		return err
	}
	defer conn.Close()

	rrq := common.RequestPacket{
		OpCode:   common.OpRRQ,
		Filename: filename,
		Mode:     "octet",
	}

	_, err = conn.WriteTo(rrq.ToBytes(), serverAddr)
	if err != nil {
		return fmt.Errorf("Error sending RRQ packet: %v", err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Error creating file: %v", err)
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	defer bw.Flush()

	// TODO: Need to read first data packet
	// and communicate on the new address
	err = common.WriteFileLoop(bw, conn, serverAddr)
	if err != nil {
		return fmt.Errorf("Error getting file: %v", err)
	}

	return nil
}

func handleState(s clientState) {
	switch s.mode {
	case modePut:
		if err := handlePut(s.filename, s.address); err != nil {
			log.Printf("Error performing put: %v", err)
		}

	case modeGet:
		if err := handleGet(s.filename, s.address); err != nil {
			log.Printf("Error performing get: %v", err)
		}
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
