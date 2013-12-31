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
	blockSize     = 512
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

	modeString := string(mode[:len(mode)-1])
	if modeString != "netascii" && modeString != "octet" {
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

func writeAck(packet []byte, tid uint16) error {
	if len(packet) != 4 {
		return fmt.Errorf("Expected slice of length 4")
	}
	binary.BigEndian.PutUint16(packet, uint16(OpACK))
	binary.BigEndian.PutUint16(packet[2:4], tid)
	return nil
}

func handleWriteRequest(remoteAddress *net.UDPAddr, filename string) {
	log.Println("Handling WRQ")
	// Don't use DialUDP here, see https://groups.google.com/forum/#!topic/golang-nuts/Mb3MS9Khito
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		log.Println(err)
		return
	}

	f, err := os.Create(filename)
	if err != nil {
		log.Println(err)
		_, writeErr := conn.WriteToUDP(createErrorPacket(0, err.Error()), remoteAddress)
		if writeErr != nil {
			log.Println(writeErr)
		}
		return
	}
	defer f.Close()

	tid := uint16(0)

	// Acknowledge WRQ

	// Check for write errors
	ack := make([]byte, 4)
	err = writeAck(ack, tid)
	if err != nil {
		log.Println(err)
		return
	}
	_, err = conn.WriteToUDP(ack, remoteAddress)
	if err != nil {
		log.Println(err)
		return
	}

	packet := make([]byte, maxPacketSize)

	for {
		tid++

		// Read data packet
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

		packetTID := binary.BigEndian.Uint16(packet[2:4])
		if packetTID != tid {
			log.Println("Expected TID %d, got %d", tid, packetTID)
			_, err := conn.WriteToUDP(createErrorPacket(5, "Unknown transfer id"), remoteAddress)
			if err != nil {
				log.Println(err)
			}
			return
		}

		// Write data
		_, err = f.Write(packet[4:n])
		if err != nil {
			log.Println(err)
			return
		}

		// Check for write errors
		err = writeAck(ack, tid)
		if err != nil {
			log.Println(err)
			return
		}
		_, err = conn.WriteToUDP(ack, remoteAddress)
		if err != nil {
			log.Println(err)
			return
		}

		if n < 4+blockSize {
			// We're done
			log.Println("Succesfully received", filename)
			return
		}
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
