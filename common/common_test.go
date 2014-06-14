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
