package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net"
	"reflect"
	"testing"
	"time"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

type mockAddr struct{}

func (m mockAddr) Network() string {
	return "udp"
}

func (m mockAddr) String() string {
	return "mockAddr"
}

type mockPacketConn struct {
	data *bytes.Buffer
	addr net.Addr
}

func (m *mockPacketConn) ReadFrom(b []byte) (n int, add net.Addr, err error) {
	to := bytes.NewBuffer(b)
	to.Truncate(0)
	n64, err := io.Copy(to, m.data)
	return int(n64), m.addr, err
}

func (m *mockPacketConn) WriteTo(b []byte, add net.Addr) (n int, err error) {
	from := bytes.NewReader(b)
	n64, err := io.Copy(m.data, from)
	return int(n64), err
}

func (m *mockPacketConn) Close() error {
	return nil
}

func (m *mockPacketConn) LocalAddr() net.Addr {
	return m.addr
}

func (m *mockPacketConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockPacketConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockPacketConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func sampleRRQ() []byte {
	return []byte{0, 1, 'H', 'e', 'l', 'l', 'o', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0}
}

func sampleWRQ() []byte {
	return []byte{0, 2, 'H', 'e', 'l', 'l', 'o', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0}
}

func TestHandleHandshakeWithRRQ(t *testing.T) {
	conn := &mockPacketConn{
		data: &bytes.Buffer{},
		addr: mockAddr{},
	}

	_, err := conn.data.Write(sampleRRQ())
	if err != nil {
		t.Fatal(err)
	}

	err = handleHandshake(conn)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHandleHandshakeWithWRQ(t *testing.T) {
	conn := &mockPacketConn{
		data: &bytes.Buffer{},
		addr: mockAddr{},
	}

	_, err := conn.data.Write(sampleWRQ())
	if err != nil {
		t.Fatal(err)
	}

	err = handleHandshake(conn)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseRequestPacket(t *testing.T) {
	testCases := []struct {
		packet         []byte
		expectedPacket *RequestPacket
		shouldError    bool
	}{
		// Nil packet
		{
			packet:         nil,
			expectedPacket: nil,
			shouldError:    true,
		},
		// Empty packet
		{
			packet:         []byte{},
			expectedPacket: nil,
			shouldError:    true,
		},
		// RRQ
		{
			packet: []byte{0, 1, 'H', 'e', 'l', 'l', 'o', 0, 'M', 'o', 'd', 'e', 0},
			expectedPacket: &RequestPacket{
				OpCode:   OpRRQ,
				Filename: "Hello",
				Mode:     "Mode",
			},
			shouldError: false,
		},
		// WRQ
		{
			packet: []byte{0, 2, 66, 0, 66, 0},
			expectedPacket: &RequestPacket{
				OpCode:   OpWRQ,
				Filename: "B",
				Mode:     "B",
			},
			shouldError: false,
		},
		// Invalid name
		{
			packet:         []byte{0, 1, 'H', 'e', 'l', 'l', 'o'},
			expectedPacket: nil,
			shouldError:    true,
		},
		// Invalid mode
		{
			packet:         []byte{0, 1, 'H', 'e', 'l', 'l', 'o', 0, 'A'},
			expectedPacket: nil,
			shouldError:    true,
		},
		// Invalid opcode
		{
			packet:         []byte{1, 1, 'H', 'e', 'l', 'l', 'o'},
			expectedPacket: nil,
			shouldError:    true,
		},
	}

	for i, tc := range testCases {
		packet, err := parseRequestPacket(tc.packet)
		if tc.shouldError && err == nil {
			t.Errorf("Expected error, didn't get one (%d)", i)
		}
		if !tc.shouldError && err != nil {
			t.Errorf("%v (%d)", err)
		}
		if !reflect.DeepEqual(tc.expectedPacket, packet) {
			t.Errorf("Expected")
			t.Errorf("%v", tc.expectedPacket)
			t.Errorf("Got")
			t.Errorf("%v", packet)
		}
	}
}

func TestCreateErrorPacket(t *testing.T) {
	p, err := createErrorPacket(2, "Hello")
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{0, 5, 0, 2, 72, 101, 108, 108, 111, 0}
	if !reflect.DeepEqual(p, expected) {
		t.Errorf("Expected")
		t.Errorf("%v", expected)
		t.Errorf("Got")
		t.Errorf("%v", p)
	}
}

func TestGetOpcode(t *testing.T) {
	testCases := []struct {
		data           []byte
		expectedOpcode OpCode
		shouldError    bool
	}{
		// Standard RRQ
		{
			data:           []byte{0, 1},
			expectedOpcode: OpRRQ,
			shouldError:    false,
		},
		// Empty data
		{
			data:           []byte{},
			expectedOpcode: OpERROR,
			shouldError:    true,
		},
		// Unknown opcode
		{
			data:           []byte{0, 99},
			expectedOpcode: OpERROR,
			shouldError:    true,
		},
		// Only 1 byte
		{
			data:           []byte{1},
			expectedOpcode: OpERROR,
			shouldError:    true,
		},
		// More than 2 bytes
		{
			data:           []byte{0, 1, 2},
			expectedOpcode: OpRRQ,
			shouldError:    false,
		},
	}

	for i, tc := range testCases {
		oc, err := getOpCode(tc.data)
		if tc.shouldError && err == nil {
			t.Errorf("Expected error, didn't get one (%d)", i)
			continue
		}
		if !tc.shouldError && err != nil {
			t.Errorf("%v (%d)", err)
			continue
		}
		if oc != tc.expectedOpcode {
			t.Errorf("Expected: %v, got %v (%d)", tc.expectedOpcode, oc, i)
		}
	}
}
