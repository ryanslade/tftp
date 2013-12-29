package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
)

type OpCode uint16

const (
	OpRRQ   OpCode = 1
	OpWRQ   OpCode = 2
	OpDATA  OpCode = 3
	OpACK   OpCode = 4
	OpERROR OpCode = 5
)

const (
	maxPacketSize = 2048
)

type RequestPacket struct {
	OpCode   OpCode
	FileName string
	Mode     string
}

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

func waitForRequest(addr *net.UDPAddr) (*RequestPacket, *net.UDPAddr, error) {
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	packet := make([]byte, maxPacketSize)
	n, remoteAddr, err := conn.ReadFromUDP(packet)
	if err != nil {
		return nil, nil, err
	}
	if n == maxPacketSize {
		return nil, nil, fmt.Errorf("Packet too big: %d bytes", n)
	}

	// Get opcode
	opcode, err := getOpCode(packet)
	if err != nil {
		return nil, remoteAddr, err
	}

	// Get filename
	reader := bytes.NewBuffer(packet[2:])
	filename, err := reader.ReadBytes(byte(0))
	if err != nil {
		return nil, remoteAddr, fmt.Errorf("Error reading filename: %v", err)
	}

	// Get mode
	mode, err := reader.ReadBytes(byte(0))
	if err != nil {
		return nil, remoteAddr, fmt.Errorf("Error reading mode: %v", err)
	}

	request := &RequestPacket{
		OpCode:   opcode,
		FileName: string(filename),
		Mode:     string(mode),
	}

	return request, remoteAddr, nil
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", ":4567")
	if err != nil {
		log.Println(err)
		return
	}
	for {
		log.Println("Waiting for request")
		request, remoteAddr, err := waitForRequest(addr)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Printf("Request: %+v from %s", request, remoteAddr.String())
	}
}
