package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"reflect"
	"testing"
	"time"
)

func init() {
	log.SetOutput(ioutil.Discard)
	handlerMapping = map[OpCode]requestHandler{}
}

func sampleRRQ() []byte {
	return []byte{0, 1, 'H', 'e', 'l', 'l', 'o', 'R', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0}
}

func sampleWRQ() []byte {
	return []byte{0, 2, 'H', 'e', 'l', 'l', 'o', 'W', 0, 'n', 'e', 't', 'a', 's', 'c', 'i', 'i', 0}
}

func TestAcceptedMode(t *testing.T) {
	testCases := []struct {
		mode     string
		accepted bool
	}{
		// Three accepted modes
		{mode: "netascii", accepted: true},
		{mode: "octet", accepted: true},
		{mode: "mail", accepted: true},

		// Mixed case should be allowed
		{mode: "netAscii", accepted: true},
		{mode: "OcteT", accepted: true},
		{mode: "Mail", accepted: true},

		// Anything else should be rejected
		{mode: "", accepted: false},
		{mode: "mode", accepted: false},
		{mode: "blah", accepted: false},
	}

	for _, tc := range testCases {
		outcome := acceptedMode(tc.mode)
		if outcome != tc.accepted {
			t.Errorf("Expected mode, '%s' accepted = %v", tc.mode, tc.accepted)
		}
	}
}

// Make sure the correct handler is called
func TestHandleHandshake(t *testing.T) {
	testCases := []struct {
		opCode           OpCode
		req              []byte
		expectedFileName string
	}{
		{
			opCode:           OpRRQ,
			req:              sampleRRQ(),
			expectedFileName: "HelloR",
		},
		{
			opCode:           OpWRQ,
			req:              sampleWRQ(),
			expectedFileName: "HelloW",
		},
	}

	rChan := make(chan struct{})
	mockRRQHandler := &mockHandler{
		replyChan: rChan,
	}

	wChan := make(chan struct{})
	mockWRQHandler := &mockHandler{
		replyChan: wChan,
	}
	handlerMapping[OpRRQ] = mockRRQHandler
	handlerMapping[OpWRQ] = mockWRQHandler

	for i, tc := range testCases {
		conn := &mockPacketConn{
			data: &bytes.Buffer{},
			addr: mockAddr{},
		}

		_, err := conn.data.Write(tc.req)
		if err != nil {
			t.Log(i)
			t.Fatal(err)
		}

		err = handleHandshake(conn)
		if err != nil {
			t.Log(i)
			t.Fatal(err)
		}

		// Wait for the replyChan in the mock since the handler is spawned
		// in another goroutine
		var waitChan chan struct{}
		switch tc.opCode {
		case OpRRQ:
			waitChan = rChan
		case OpWRQ:
			waitChan = wChan
		}
		select {
		case <-waitChan:
			// All good
		case <-time.After(1 * time.Millisecond):
			t.Errorf("Didn't receive, handler not called (%d)", i)
		}
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
			t.Errorf("%v (%d)", err, i)
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
			t.Errorf("%v (%d)", err, i)
			continue
		}
		if oc != tc.expectedOpcode {
			t.Errorf("Expected: %v, got %v (%d)", tc.expectedOpcode, oc, i)
		}
	}
}

func BenchmarkCreateErrorPacket(b *testing.B) {
	for i := 0; i < b.N; i++ {
		packet, err := createErrorPacket(1, "Error")
		if err != nil {
			b.Fatal(err)
		}
		if len(packet) == 0 {
			b.Fatal("Packet is empty")
		}
	}
}
