package main

import (
	"bufio"
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
	defer conn.Close()

	f, err := os.Open(filename)
	if err != nil {
		log.Println(err)
		if os.IsNotExist(err) {
			common.SendError(1, "File not found", conn, remoteAddress)
			return
		}
		common.SendError(0, err.Error(), conn, remoteAddress)
		return
	}
	defer f.Close()

	br := bufio.NewReader(f)
	bytesRead, err := common.ReadFileLoop(br, conn, remoteAddress, common.BlockSize)
	if err != nil {
		log.Println("Error handling read:", err)
	}
	log.Printf("Done sending %s. %d bytes in %v", filename, bytesRead, time.Since(start))
}

func fileCleanup(f *os.File) {
	if err := f.Sync(); err != nil {
		log.Printf("Error syncing %s, %v", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		log.Printf("Error closing file %s, %v", f.Name(), err)
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
	defer conn.Close()

	f, err := os.Create(filename)
	if err != nil {
		log.Println(err)
		// TODO: This error should indicate what went wrong
		common.SendError(0, err.Error(), conn, remoteAddress)
		return
	}
	defer fileCleanup(f)

	bw := bufio.NewWriter(f)
	defer bw.Flush()

	tid := uint16(0)

	// Acknowledge WRQ
	ack := common.CreateAckPacket(tid)
	_, err = conn.WriteTo(ack, remoteAddress)
	if err != nil {
		log.Println(err)
		return
	}

	err = common.WriteFileLoop(bw, conn, remoteAddress)
	if err != nil {
		log.Println("Error sending file:", err)
	}
	log.Println("Seccesfully received:", filename)
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
