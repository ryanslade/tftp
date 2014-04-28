package main

import (
	"reflect"
	"testing"
)

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
