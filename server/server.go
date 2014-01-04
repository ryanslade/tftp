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
	blockSize     = 512
	maxPacketSize = blockSize * 2
)

func getOpCode(packet []byte) (OpCode, error) {
	if len(packet) < 2 {
		return OpERROR, fmt.Errorf("Packet too small to get opcode")
	}
	opcode := OpCode(binary.BigEndian.Uint16(packet))
	if opcode > 5 {
		return OpERROR, fmt.Errorf("Unknown opcode: %d", opcode)
	}
	return opcode, nil
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
	// Remove trailing 0
	filename = filename[:len(filename)-1]

	// Get mode
	mode, err := reader.ReadBytes(byte(0))
	if err != nil {
		return fmt.Errorf("Error reading mode: %v", err)
	}
	// Remove trailing 0
	mode = mode[:len(mode)-1]

	modeString := string(mode)
	if modeString != "netascii" && modeString != "octet" {
		return fmt.Errorf("Unknown mode: %v", string(mode))
	}

	switch opcode {
	case OpRRQ:
		go handleReadRequest(remoteAddr, string(filename))
	case OpWRQ:
		go handleWriteRequest(remoteAddr, string(filename))
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
func createErrorPacket(code uint16, message string) ([]byte, error) {
	packet := new(bytes.Buffer)
	// Check for errors
	data := []interface{}{
		OpERROR,
		code,
		[]byte(message),
		byte(0),
	}
	for _, v := range data {
		if err := binary.Write(packet, binary.BigEndian, v); err != nil {
			return nil, err
		}
	}
	return packet.Bytes(), nil
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
		errPacket, err := createErrorPacket(0, err.Error())
		if err != nil {
			log.Println(err)
			return
		}
		_, err = conn.WriteToUDP(errPacket, remoteAddress)
		if err != nil {
			log.Println(err)
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
			errPacket, err := createErrorPacket(5, "Unknown transfer id")
			if err != nil {
				log.Println(err)
				return
			}
			_, err = conn.WriteToUDP(errPacket, remoteAddress)
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

	log.Println("Waiting for request")
	for {
		err := readRequest(conn)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}
