package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ryanslade/tftp/common"
)

// Flags
var (
	port int
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

func acceptedMode(mode string) bool {
	switch strings.ToLower(mode) {
	case "netascii", "octet", "mail":
		return true
	}
	return false
}

func handleHandshake(conn net.PacketConn) error {
	packet := make([]byte, common.MaxPacketSize)

	n, remoteAddr, err := conn.ReadFrom(packet)
	if err != nil {
		return fmt.Errorf("Error reading from connection: %v", err)
	}
	if n == common.MaxPacketSize {
		return fmt.Errorf("Packet too big: %d bytes", n)
	}

	log.Printf("Request from %v", remoteAddr)
	req, err := common.ParseRequestPacket(packet)
	if err != nil {
		return fmt.Errorf("Error parsing request packet: %v", err)
	}

	if !acceptedMode(req.Mode) {
		return fmt.Errorf("Unknown mode: %s", req.Mode)
	}

	handler, ok := handlerMapping[req.OpCode]
	if !ok {
		return fmt.Errorf("No handler for OpCode: %d\n", req.OpCode)
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

func handleReadRequest(remoteAddress net.Addr, filename string) {
	start := time.Now()
	log.Println("Handling RRQ for", filename)

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
	err = common.ReadFileLoop(br, conn, remoteAddress, common.BlockSize)
	if err != nil {
		log.Println("Error handling read:", err)
	}
	log.Printf("Done sending %s. (%v)", filename, time.Since(start))
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

	packet := make([]byte, common.MaxPacketSize)

	for {
		tid++

		// Read data packet
		n, _, err := conn.ReadFromUDP(packet)
		if err != nil {
			log.Println(err)
			return
		}

		opcode, err := common.GetOpCode(packet)
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

		if n < 4+common.BlockSize {
			// We're done
			log.Println("Succesfully received", filename)
			return
		}
	}
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

func init() {
	flag.IntVar(&port, "port", 69, "Port to listen on")
}

func main() {
	flag.Parse()
	listenAndServe(port)
}
