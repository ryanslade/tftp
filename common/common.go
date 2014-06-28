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

func SendError(code uint16, message string, conn net.PacketConn, remoteAddress net.Addr) error {
	errPacket := CreateErrorPacket(0, message)
	_, err := conn.WriteTo(errPacket, remoteAddress)
	if err != nil {
		return fmt.Errorf("Error writing error packet: %v", err)
	}
	return nil
}

// writes an ack packet to the supplied byte slice
//
//  2 bytes     2 bytes
//  ---------------------
// | Opcode |   Block #  |
//  ---------------------
func CreateAckPacket(tid uint16) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf, uint16(OpACK))
	binary.BigEndian.PutUint16(buf[2:], tid)
	return buf
}

// creates an error packet with the following structure:
//
// 2 bytes     2 bytes      string    1 byte
// -----------------------------------------
// | Opcode |  ErrorCode |   ErrMsg   |   0  |
// -----------------------------------------
func CreateErrorPacket(code uint16, message string) []byte {
	buf := make([]byte, 2+2+len(message)+1)
	binary.BigEndian.PutUint16(buf, uint16(OpERROR)) // 2 bytes
	binary.BigEndian.PutUint16(buf[2:], code)        // 2 bytes
	copy(buf[4:], []byte(message))
	buf[len(buf)-1] = byte(0)
	return buf
}

func WriteFileLoop(w io.Writer, conn net.PacketConn, remoteAddress net.Addr) error {
	// Assume we have already sent the initial ACK packet
	tid := uint16(0)
	packet := make([]byte, MaxPacketSize)
	for {
		tid++

		// Read data packet
		n, _, err := conn.ReadFrom(packet)
		if err != nil {
			return fmt.Errorf("Error reading packet: %v", err)
		}

		opcode, err := GetOpCode(packet)
		if err != nil {
			return fmt.Errorf("Error getting opcode: %v", err)
		}
		if opcode != OpDATA {
			return fmt.Errorf("Expected DATA packet, got %v\n", opcode)
		}

		packetTID := binary.BigEndian.Uint16(packet[2:4])
		if packetTID != tid {
			SendError(5, "Unknown transfer id", conn, remoteAddress)
			return fmt.Errorf("Expected TID %d, got %d\n", tid, packetTID)
		}

		// Write data to disk
		_, err = w.Write(packet[4:n])
		if err != nil {
			return fmt.Errorf("Error writing: %v", err)
		}

		ack := CreateAckPacket(tid)
		_, err = conn.WriteTo(ack, remoteAddress)
		if err != nil {
			return fmt.Errorf("Error writing ACK packet: %v", err)
		}

		if n < 4+BlockSize {
			return nil
		}
	}
}

// ReadFileLoop will read from r in blockSize chunks, sending each chunk to through conn
// to remoteAddr. After each send it will wait for an ACK packet. It will loop until
// EOF on r.
func ReadFileLoop(r io.Reader, conn net.PacketConn, remoteAddr net.Addr, blockSize int) (int, error) {
	var tid uint16
	var bytesRead int

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
			return bytesRead, fmt.Errorf("Error reading data: %v", err)
		}
		bytesRead += n

		packet := createDataPacket(tid, buffer[:n])
		n, err = conn.WriteTo(packet, remoteAddr)
		if err != nil {
			return bytesRead, fmt.Errorf("Error writing data packet: %v", err)
		}

		// Read ack
		i, _, err := conn.ReadFrom(ackBuf)
		if err != nil {
			return bytesRead, fmt.Errorf("Error reading ACK packet: %v", err)
		}
		if i != 4 {
			return bytesRead, fmt.Errorf("Expected 4 bytes read for ACK packet, got %d", i)
		}
		ackTid, err := ParseAckPacket(ackBuf)
		if err != nil {
			return bytesRead, fmt.Errorf("Error parsing ACK packet: %v", err)
		}
		if ackTid != tid {
			return bytesRead, fmt.Errorf("ACK tid: %d, does not match expected: %d", ackTid, tid)
		}
	}
	return bytesRead, nil
}
