package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/ryanslade/tftp/common"
)

func init() {
	log.SetOutput(ioutil.Discard)
	handlerMapping = map[common.OpCode]requestHandler{}
}

func TestParseACKPacket(t *testing.T) {
	testCases := []struct {
		packet      []byte
		tid         uint16
		errExpected bool
	}{
		// Valid packet
		{
			packet:      []byte{0, 4, 0, 1},
			tid:         1,
			errExpected: false,
		},
		// Wrong opcode
		{
			packet:      []byte{0, 3, 0, 1},
			tid:         1,
			errExpected: true,
		},
	}

	for i, tc := range testCases {
		tid, err := common.ParseAckPacket(tc.packet)
		if tc.errExpected && err == nil {
			t.Errorf("Expected an error, got nil (%d)", i)
			continue
		}
		if !tc.errExpected && err != nil {
			t.Errorf("Error: %v (%d)", err, i)
			continue
		}
		if tc.errExpected && err != nil {
			continue
		}
		if tid != tc.tid {
			t.Errorf("Expected tid: %d, got %d (%d)", tc.tid, tid, i)
		}
	}
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
		opCode           common.OpCode
		req              []byte
		expectedFileName string
	}{
		{
			opCode:           common.OpRRQ,
			req:              sampleRRQ(),
			expectedFileName: "HelloR",
		},
		{
			opCode:           common.OpWRQ,
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
	handlerMapping[common.OpRRQ] = mockRRQHandler
	handlerMapping[common.OpWRQ] = mockWRQHandler

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
		case common.OpRRQ:
			waitChan = rChan
		case common.OpWRQ:
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

func TestGetOpcode(t *testing.T) {
	testCases := []struct {
		data           []byte
		expectedOpcode common.OpCode
		shouldError    bool
	}{
		// Standard RRQ
		{
			data:           []byte{0, 1},
			expectedOpcode: common.OpRRQ,
			shouldError:    false,
		},
		// Empty data
		{
			data:           []byte{},
			expectedOpcode: common.OpERROR,
			shouldError:    true,
		},
		// Unknown opcode
		{
			data:           []byte{0, 99},
			expectedOpcode: common.OpERROR,
			shouldError:    true,
		},
		// Only 1 byte
		{
			data:           []byte{1},
			expectedOpcode: common.OpERROR,
			shouldError:    true,
		},
		// More than 2 bytes
		{
			data:           []byte{0, 1, 2},
			expectedOpcode: common.OpRRQ,
			shouldError:    false,
		},
	}

	for i, tc := range testCases {
		oc, err := common.GetOpCode(tc.data)
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
