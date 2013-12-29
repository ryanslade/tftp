package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
)

const (
	OpRRQ   = 1
	OpWRQ   = 2
	OpDATA  = 3
	OpACK   = 4
	OpERROR = 5

	maxPacketSize = 2048
)

func listen(addr *net.UDPAddr) error {
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	packet := make([]byte, maxPacketSize)
	n, remoteAddr, err := conn.ReadFromUDP(packet)
	if err != nil {
		return err
	}
	if n == maxPacketSize {
		return fmt.Errorf("Packet too big: %d bytes", n)
	}
	fmt.Println(remoteAddr)

	// Get opcode
	opcode := binary.BigEndian.Uint16(packet)
	log.Println("Opcode:", opcode)

	// Get filename
	reader := bytes.NewBuffer(packet[2:])
	filename, err := reader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Error reading filename: %v", err)
	}
	log.Println("Filename:", string(filename))

	// Get mode
	mode, err := reader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Error reading mode: %v", err)
	}
	log.Println("Mode:", string(mode))

	return nil
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", ":4567")
	if err != nil {
		log.Println(err)
		return
	}
	for {
		log.Println("Listening")
		err := listen(addr)
		if err != nil {
			log.Println(err)
		}
	}
}
