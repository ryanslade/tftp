package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/ryanslade/tftp/common"
)

// Flags
var (
	port int
)

const (
	blockSize     = 512
	maxPacketSize = blockSize * 2
)

type requestHandler interface {
	serve(remoteAddr net.Addr, filename string)
}

type requestHandlerFunc func(remoteAddr net.Addr, filename string)

func (r requestHandlerFunc) serve(remoteAddr net.Addr, filename string) {
	r(remoteAddr, filename)
}

var handlerMapping = map[common.OpCode]requestHandler{
	common.OpRRQ: requestHandlerFunc(handleReadRequest),
	common.OpWRQ: requestHandlerFunc(handleWriteRequest),
}

func getOpCode(packet []byte) (common.OpCode, error) {
	if len(packet) < 2 {
		return common.OpERROR, fmt.Errorf("Packet too small to get opcode")
	}
	opcode := common.OpCode(binary.BigEndian.Uint16(packet))
	if opcode > 5 {
		return common.OpERROR, fmt.Errorf("Unknown opcode: %d", opcode)
	}
	return opcode, nil
}

// parses a request packet in the form:
//
//  2 bytes     string    1 byte     string   1 byte
// ------------------------------------------------
// | Opcode |  Filename  |   0  |    Mode    |   0  |
// ------------------------------------------------
func parseRequestPacket(packet []byte) (*common.RequestPacket, error) {
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

	return &common.RequestPacket{
		OpCode:   opcode,
		Mode:     string(mode),
		Filename: string(filename),
	}, nil
}

func acceptedMode(mode string) bool {
	switch strings.ToLower(mode) {
	case "netascii", "octet", "mail":
		return true
	}
	return false
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

	if !acceptedMode(req.Mode) {
		return fmt.Errorf("Unknown mode: %s", req.Mode)
	}

	handler, ok := handlerMapping[req.OpCode]
	if !ok {
		log.Printf("No handler for OpCode: %d\n", req.OpCode)
	}
	go handler.serve(remoteAddr, req.Filename)

	return nil
}

// creates an error packet with the following structure:
//
// 2 bytes     2 bytes      string    1 byte
// -----------------------------------------
// | Opcode |  ErrorCode |   ErrMsg   |   0  |
// -----------------------------------------
func createErrorPacket(code uint16, message string) []byte {
	buf := make([]byte, 2+2+len(message)+1)
	binary.BigEndian.PutUint16(buf, uint16(common.OpERROR)) // 2 bytes
	binary.BigEndian.PutUint16(buf[2:], code)               // 2 bytes
	copy(buf[4:], []byte(message))
	buf[len(buf)-1] = byte(0)
	return buf
}

//  2 bytes     2 bytes      n bytes
//  ----------------------------------
// | Opcode |   Block #  |   Data     |
//  ----------------------------------
func createDataPacket(blockNumber uint16, data []byte) []byte {
	buf := make([]byte, 2+2+len(data))
	binary.BigEndian.PutUint16(buf, uint16(common.OpDATA))
	binary.BigEndian.PutUint16(buf[2:], blockNumber)
	copy(buf[4:], data)
	return buf
}

// writes an ack packet to the supplied byte slice
//
//  2 bytes     2 bytes
//  ---------------------
// | Opcode |   Block #  |
//  ---------------------
func createAckPacket(tid uint16) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf, uint16(common.OpACK))
	binary.BigEndian.PutUint16(buf[2:], tid)
	return buf
}

//  2 bytes     2 bytes
//  ---------------------
// | Opcode |   Block #  |
//  ---------------------
func parseAckPacket(packet []byte) (tid uint16, err error) {
	op, err := getOpCode(packet)
	if err != nil {
		return 0, fmt.Errorf("Error getting opcode: %v", err)
	}
	if op != common.OpACK {
		return 0, fmt.Errorf("Expected ACK packet, got OpCode: %d", op)
	}
	tid = binary.BigEndian.Uint16(packet[2:])
	return tid, nil
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

	br := bufio.NewReader(f)

	var tid uint16
	buffer := make([]byte, blockSize)
	ackBuf := make([]byte, 4)
	for {
		tid++

		n, err := br.Read(buffer)
		if err == io.EOF {
			log.Println("Done sending", filename)
			break
		}
		if err != nil {
			log.Println("Error reading file:", err)
			break
		}

		packet := createDataPacket(tid, buffer[:n])
		n, err = conn.WriteTo(packet, remoteAddress)
		if err != nil {
			log.Println("Error writing data packet:", err)
			sendError(0, err.Error(), conn, remoteAddress)
			break
		}

		// Read ack
		i, _, err := conn.ReadFrom(ackBuf)
		if err != nil {
			log.Println("Error reading ACK packet:", err)
			break
		}
		if i != 4 {
			log.Println("Expected 4 bytes read for ACK packet, got", i)
			break
		}
		ackTid, err := parseAckPacket(ackBuf)
		if err != nil {
			log.Println("Error parsing ACK packet:", err)
			break
		}
		if ackTid != tid {
			log.Printf("ACK tid: %d, does not match expected: %d", ackTid, tid)
			break
		}
	}
}

func sendError(code uint16, message string, conn net.PacketConn, remoteAddress net.Addr) {
	errPacket := createErrorPacket(0, message)
	_, err := conn.WriteTo(errPacket, remoteAddress)
	if err != nil {
		log.Println("Error writing error packet:", err)
	}
	return
}

func fileCleanup(f *os.File) {
	if err := f.Sync(); err != nil {
		log.Println("Error syncing %s, %v", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		log.Println("Error closing file %s, %v", f.Name(), err)
	}
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
	defer fileCleanup(f)

	bw := bufio.NewWriter(f)
	defer bw.Flush()

	tid := uint16(0)

	// Acknowledge WRQ
	ack := createAckPacket(tid)
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
		if opcode != common.OpDATA {
			log.Printf("Expected DATA packet, got %v\n", opcode)
			return
		}

		packetTID := binary.BigEndian.Uint16(packet[2:4])
		if packetTID != tid {
			log.Printf("Expected TID %d, got %d\n", tid, packetTID)
			sendError(5, "Unknown transfer id", conn, remoteAddress)
			return
		}

		// Write data to disk
		_, err = bw.Write(packet[4:n])
		if err != nil {
			log.Println(err)
			return
		}

		ack := createAckPacket(tid)
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

func listenAndServe(port int) {
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

func main() {
	flag.Parse()
	listenAndServe(port)
}
