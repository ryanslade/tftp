package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	BlockSize     = 512
	MaxPacketSize = BlockSize * 2
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

type RequestPacket struct {
	OpCode   OpCode
	Filename string
	Mode     string
}

//  2 bytes     2 bytes      n bytes
//  ----------------------------------
// | Opcode |   Block #  |   Data     |
//  ----------------------------------
func createDataPacket(blockNumber uint16, data []byte) []byte {
	buf := make([]byte, 2+2+len(data))
	binary.BigEndian.PutUint16(buf, uint16(OpDATA))
	binary.BigEndian.PutUint16(buf[2:], blockNumber)
	copy(buf[4:], data)
	return buf
}

//  2 bytes     2 bytes
//  ---------------------
// | Opcode |   Block #  |
//  ---------------------
func ParseAckPacket(packet []byte) (tid uint16, err error) {
	op, err := GetOpCode(packet)
	if err != nil {
		return 0, fmt.Errorf("Error getting opcode: %v", err)
	}
	if op != OpACK {
		return 0, fmt.Errorf("Expected ACK packet, got OpCode: %d", op)
	}
	tid = binary.BigEndian.Uint16(packet[2:])
	return tid, nil
}

// parses a request packet in the form:
//
//  2 bytes     string    1 byte     string   1 byte
// ------------------------------------------------
// | Opcode |  Filename  |   0  |    Mode    |   0  |
// ------------------------------------------------
func ParseRequestPacket(packet []byte) (*RequestPacket, error) {
	// Get opcode
	opcode, err := GetOpCode(packet)
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

func (p RequestPacket) ToBytes() []byte {
	buf := make([]byte, 2+len(p.Filename)+1+len(p.Mode)+1)
	binary.BigEndian.PutUint16(buf, uint16(p.OpCode))
	copy(buf[2:], p.Filename)
	copy(buf[2+len(p.Filename)+1:], p.Mode)
	return buf
}

// GetOpCode will attempt to parse the OpCode from the packet passed in
func GetOpCode(packet []byte) (OpCode, error) {
	if len(packet) < 2 {
		return OpERROR, fmt.Errorf("Packet too small to get opcode")
	}
	opcode := OpCode(binary.BigEndian.Uint16(packet))
	if opcode > 5 {
		return OpERROR, fmt.Errorf("Unknown opcode: %d", opcode)
	}
	return opcode, nil
}

// ReadLoop will read from r in blockSize chunks, sending each chunk to through conn
// to remoteAddr. After each send it will wait for an ACK packet. It will loop until
// EOF on r.
func ReadLoop(r io.Reader, conn net.PacketConn, remoteAddr net.Addr, blockSize int) error {
	var tid uint16
	buffer := make([]byte, blockSize)
	ackBuf := make([]byte, 4)
	for {
		tid++

		n, err := r.Read(buffer)
		if err == io.EOF {
			// We're done
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading data: %v", err)
		}

		packet := createDataPacket(tid, buffer[:n])
		n, err = conn.WriteTo(packet, remoteAddr)
		if err != nil {
			return fmt.Errorf("Error writing data packet: %v", err)
		}

		// Read ack
		i, _, err := conn.ReadFrom(ackBuf)
		if err != nil {
			return fmt.Errorf("Error reading ACK packet: %v", err)
		}
		if i != 4 {
			return fmt.Errorf("Expected 4 bytes read for ACK packet, got %d", i)
		}
		ackTid, err := ParseAckPacket(ackBuf)
		if err != nil {
			return fmt.Errorf("Error parsing ACK packet: %v", err)
		}
		if ackTid != tid {
			return fmt.Errorf("ACK tid: %d, does not match expected: %d", ackTid, tid)
		}
	}
	return nil
}
