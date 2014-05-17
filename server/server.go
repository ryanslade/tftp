package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

// Flags
var (
	port int
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

var acceptedModes = map[string]bool{
	"netascii": true,
	"octet":    true,
}

type RequestPacket struct {
	OpCode   OpCode
	Filename string
	Mode     string
}

type requestHandler interface {
	serve(remoteAddr net.Addr, filename string)
}

type requestHandlerFunc func(remoteAddr net.Addr, filename string)

func (r requestHandlerFunc) serve(remoteAddr net.Addr, filename string) {
	r(remoteAddr, filename)
}

var handlerMapping = map[OpCode]requestHandler{
	OpRRQ: requestHandlerFunc(handleReadRequest),
	OpWRQ: requestHandlerFunc(handleWriteRequest),
}

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

// parses a request packet in the form:
//
//  2 bytes     string    1 byte     string   1 byte
// ------------------------------------------------
// | Opcode |  Filename  |   0  |    Mode    |   0  |
// ------------------------------------------------
func parseRequestPacket(packet []byte) (*RequestPacket, error) {
	// Get opcode
	opcode, err := getOpCode(packet)
	if err != nil {
		return nil, err
	}

	// Get filename
	reader := bytes.NewBuffer(packet[2:])

	filename, err := reader.ReadBytes(byte(0))
	if err != nil {
		return nil, fmt.Errorf("Error reading filename: %v", err)
	}
	// Remove trailing 0
	filename = filename[:len(filename)-1]

	// Get mode
	mode, err := reader.ReadBytes(byte(0))
	if err != nil {
		return nil, fmt.Errorf("Error reading mode: %v", err)
	}
	// Remove trailing 0
	mode = mode[:len(mode)-1]

	return &RequestPacket{
		OpCode:   opcode,
		Mode:     string(mode),
		Filename: string(filename),
	}, nil
}

func handleHandshake(conn net.PacketConn) error {
	packet := make([]byte, maxPacketSize)

	n, remoteAddr, err := conn.ReadFrom(packet)
	if err != nil {
		return fmt.Errorf("Error reading from connection: %v", err)
	}
	if n == maxPacketSize {
		return fmt.Errorf("Packet too big: %d bytes", n)
	}

	log.Printf("Request from %v", remoteAddr)
	req, err := parseRequestPacket(packet)
	if err != nil {
		return fmt.Errorf("Error parsing request packet: %v", err)
	}

	if !acceptedModes[req.Mode] {
		return fmt.Errorf("Unknown mode: %s", req.Mode)
	}

	handler, ok := handlerMapping[req.OpCode]
	if !ok {
		log.Println("No handler for opCode: %d", req.OpCode)
	}
	go handler.serve(remoteAddr, req.Filename)

	return nil
}

func createPacket(data []interface{}) ([]byte, error) {
	packet := new(bytes.Buffer)
	for _, v := range data {
		if err := binary.Write(packet, binary.BigEndian, v); err != nil {
			return nil, err
		}
	}
	return packet.Bytes(), nil
}

// creates an error packet with the following structure:
//
// 2 bytes     2 bytes      string    1 byte
// -----------------------------------------
// | Opcode |  ErrorCode |   ErrMsg   |   0  |
// -----------------------------------------
func createErrorPacket(code uint16, message string) ([]byte, error) {
	return createPacket([]interface{}{
		OpERROR,
		code,
		[]byte(message),
		byte(0),
	})
}

//  2 bytes     2 bytes      n bytes
//  ----------------------------------
// | Opcode |   Block #  |   Data     |
//  ----------------------------------
func createDataPacket(blockNumber uint16, data []byte) ([]byte, error) {
	return createPacket([]interface{}{
		OpDATA,
		blockNumber,
		data,
	})
}

// writes an ack packet to the supplied byte slice
//
//  2 bytes     2 bytes
//  ---------------------
// | Opcode |   Block #  |
//  ---------------------
func createAckPacket(tid uint16) ([]byte, error) {
	return createPacket([]interface{}{
		OpACK,
		tid,
	})
}

func handleReadRequest(remoteAddress net.Addr, filename string) {
	log.Println("Handling RRQ")

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	})
	if err != nil {
		log.Println("Error listening", err)
		return
	}

	f, err := os.Open(filename)
	if err != nil {
		log.Println(err)
		if os.IsNotExist(err) {
			sendError(1, "File not found", conn, remoteAddress)
			return
		}
		sendError(0, err.Error(), conn, remoteAddress)
		return
	}
	defer f.Close()

	var tid uint16
	buffer := make([]byte, blockSize)
	for {
		tid++

		n, err := f.Read(buffer)
		if err == io.EOF {
			break
		}
		packet, err := createDataPacket(tid, buffer[:n])
		if err != nil {
			log.Println(err)
			sendError(0, err.Error(), conn, remoteAddress)
			break
		}
		n, err = conn.WriteTo(packet, remoteAddress)
		if err != nil {
			log.Println("Error writing data packet:", err)
			sendError(0, err.Error(), conn, remoteAddress)
			break
		}
	}
}

func sendError(code uint16, message string, conn net.PacketConn, remoteAddress net.Addr) {
	errPacket, err := createErrorPacket(0, message)
	if err != nil {
		log.Println(err)
		return
	}
	_, err = conn.WriteTo(errPacket, remoteAddress)
	if err != nil {
		log.Println("Error writing error packet:", err)
	}
	return
}

func handleWriteRequest(remoteAddress net.Addr, filename string) {
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
		// TODO: This error should indicate what went wrong
		sendError(0, err.Error(), conn, remoteAddress)
		return
	}
	defer f.Close()

	tid := uint16(0)

	// Acknowledge WRQ
	ack, err := createAckPacket(tid)
	if err != nil {
		log.Println("Error creating ack packet", err)
		return
	}
	_, err = conn.WriteTo(ack, remoteAddress)
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
			sendError(5, "Unknown transfer id", conn, remoteAddress)
			return
		}

		// Write data
		_, err = f.Write(packet[4:n])
		if err != nil {
			log.Println(err)
			return
		}

		ack, err := createAckPacket(tid)
		if err != nil {
			log.Println("Error creating ACK packet:", err)
			return
		}
		_, err = conn.WriteTo(ack, remoteAddress)
		if err != nil {
			log.Println("Error writing ACK packet:", err)
			return
		}

		if n < 4+blockSize {
			// We're done
			log.Println("Succesfully received", filename)
			return
		}
	}
}

func init() {
	flag.IntVar(&port, "port", 69, "Port to listen on")
}

func main() {
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
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

	log.Println("Waiting for requests on port", port)
	for {
		err := handleHandshake(conn)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}
