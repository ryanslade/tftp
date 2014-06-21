package common

import (
	"reflect"
	"testing"
)

func TestCreateDataPacket(t *testing.T) {
	testCases := []struct {
		blockNumber uint16
		data        []byte
		expected    []byte
	}{
		{
			blockNumber: 1,
			data:        []byte{1, 2, 3, 4, 5},
			expected:    []byte{0, 3, 0, 1, 1, 2, 3, 4, 5},
		},
	}

	for i, tc := range testCases {
		packet := createDataPacket(tc.blockNumber, tc.data)
		if !reflect.DeepEqual(packet, tc.expected) {
			t.Errorf("Expected and actual packet not equal (%d)", i)
			t.Error(packet)
		}
	}
}

func TestRequestPacketToBytes(t *testing.T) {
	testCases := []struct {
		packet        RequestPacket
		expectedBytes []byte
	}{
		// RRQ
		{
			expectedBytes: []byte{0, 1, 'H', 'e', 'l', 'l', 'o', 0, 'M', 'o', 'd', 'e', 0},
			packet: RequestPacket{
				OpCode:   OpRRQ,
				Filename: "Hello",
				Mode:     "Mode",
			},
		},
		// WRQ
		{
			expectedBytes: []byte{0, 2, 'B', 0, 'B', 0},
			packet: RequestPacket{
				OpCode:   OpWRQ,
				Filename: "B",
				Mode:     "B",
			},
		},
	}

	for i, tc := range testCases {
		b := tc.packet.ToBytes()
		if !reflect.DeepEqual(tc.expectedBytes, b) {
			t.Errorf("Test case %d failed", i)
			t.Errorf("Expected")
			t.Errorf("%v", tc.expectedBytes)
			t.Errorf("Got")
			t.Errorf("%v", b)
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
		packet, err := ParseRequestPacket(tc.packet)
		if tc.shouldError && err == nil {
			t.Errorf("Expected error, didn't get one (%d)", i)
		}
		if !tc.shouldError && err != nil {
			t.Errorf("%v (%d)", err, i)
		}
		if !reflect.DeepEqual(tc.expectedPacket, packet) {
			t.Errorf("Test case %d failed", i)
			t.Errorf("Expected")
			t.Errorf("%v", tc.expectedPacket)
			t.Errorf("Got")
			t.Errorf("%v", packet)
		}
	}
}
