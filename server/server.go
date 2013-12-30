package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
)

type OpCode uint16

const (
	OpRRQ   OpCode = 1
	OpWRQ   OpCode = 2
	OpDATA  OpCode = 3
	OpACK   OpCode = 4
	OpERROR OpCode = 5
)

var OpCodeNames = map[OpCode]string{
	OpRRQ:   "RRQ",
	OpWRQ:   "WRQ",
	OpDATA:  "DATA",
	OpACK:   "ACK",
	OpERROR: "ERROR",
}

func (o OpCode) String() string {
	return OpCodeNames[o]
}

const (
	maxPacketSize = 2048
)

func getOpCode(packet []byte) (OpCode, error) {
	if len(packet) < 2 {
		return OpERROR, fmt.Errorf("Packet too small to get opcode")
	}
	opcode := binary.BigEndian.Uint16(packet)
	if opcode > 5 {
		return OpERROR, fmt.Errorf("Unknown opcode: %d", opcode)
	}
	return OpCode(opcode), nil
}

func readRequest(conn *net.UDPConn) error {
	packet := make([]byte, maxPacketSize)

	n, remoteAddr, err := conn.ReadFromUDP(packet)
	if err != nil {
		return err
	}
	if n == maxPacketSize {
		return fmt.Errorf("Packet too big: %d bytes", n)
	}

	log.Printf("Request from %v", remoteAddr)

	// Get opcode
	opcode, err := getOpCode(packet)
	if err != nil {
		return err
	}

	// Get filename
	reader := bytes.NewBuffer(packet[2:])
	filename, err := reader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Error reading filename: %v", err)
	}

	// Get mode
	mode, err := reader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Error reading mode: %v", err)
	}

	if string(mode[:len(mode)-1]) != "netascii" {
		return fmt.Errorf("Unknown mode: %v", string(mode))
	}

	switch opcode {
	case OpRRQ:
		go handleReadRequest(remoteAddr, string(filename[:len(filename)-1]))
	case OpWRQ:
		go handleWriteRequest(remoteAddr, string(filename[:len(filename)-1]))
	default:
		log.Println("Unable to handle request with opcode: %d", opcode)
	}

	return nil
}

// creates an error packet with the following structure:
//
// 2 bytes     2 bytes      string    1 byte
// -----------------------------------------
// | Opcode |  ErrorCode |   ErrMsg   |   0  |
// -----------------------------------------
func createErrorPacket(code uint16, message string) []byte {
	packet := new(bytes.Buffer)
	// Check for errors
	binary.Write(packet, binary.BigEndian, OpERROR)
	binary.Write(packet, binary.BigEndian, code)
	binary.Write(packet, binary.BigEndian, []byte(message))
	binary.Write(packet, binary.BigEndian, byte(0))
	return packet.Bytes()
}

func handleReadRequest(remoteAddress *net.UDPAddr, filename string) {
	log.Println("Handling RRQ")
}

func handleWriteRequest(remoteAddress *net.UDPAddr, filename string) {
	log.Println("Handling WRQ")
	conn, err := net.DialUDP("udp", nil, remoteAddress)
	if err != nil {
		log.Println(err)
		return
	}

	f, err := os.Create(filename)
	if err != nil {
		log.Println(err)
		_, writeErr := conn.Write(createErrorPacket(0, err.Error()))
		if writeErr != nil {
			log.Println(writeErr)
		}
		return
	}
	defer f.Close()

	tid := uint16(0)

	ack := new(bytes.Buffer)

	// Check for write errors
	binary.Write(ack, binary.BigEndian, OpACK)
	binary.Write(ack, binary.BigEndian, tid)
	_, err = conn.Write(ack.Bytes())
	if err != nil {
		log.Println(err)
		return
	}

	packet := make([]byte, maxPacketSize)
	n, _, err := conn.ReadFromUDP(packet)
	if err != nil {
		log.Println(err)
		return
	}
	opcode, err := getOpCode(packet)
	if err != nil {
		log.Println(err)
		return
	}
	if opcode != OpDATA {
		log.Println("Expected DATA packet, got %v", opcode)
		return
	}
	_, err = f.Write(packet[4 : n-1])
	if err != nil {
		log.Println(err)
		return
	}
	tid++ // TODO, check that Block number in data packet matches

	ack.Reset()
	// Check for write errors
	binary.Write(ack, binary.BigEndian, OpACK)
	binary.Write(ack, binary.BigEndian, tid)
	_, err = conn.Write(ack.Bytes())
	if err != nil {
		log.Println(err)
		return
	}
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", ":4567")
	if err != nil {
		log.Println(err)
		return
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	for {
		log.Println("Waiting for request")
		err := readRequest(conn)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}
