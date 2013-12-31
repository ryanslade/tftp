package main

import (
	"reflect"
	"testing"
)

func TestCreateErrorPacket(t *testing.T) {
	p := createErrorPacket(2, "Hello")
	expected := []byte{0, 5, 0, 2, 72, 101, 108, 108, 111, 0}
	if !reflect.DeepEqual(p, expected) {
		t.Errorf("Expected")
		t.Errorf("%v", expected)
		t.Errorf("Got")
		t.Errorf("%v", p)
	}
}

func TestWriteAck(t *testing.T) {
	s := make([]byte, 4)
	err := writeAck(s, 257)
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte{0, 4, 1, 1}
	if !reflect.DeepEqual(expected, s) {
		t.Errorf("Expected")
		t.Errorf("%v", expected)
		t.Errorf("Got")
		t.Errorf("%v", s)
	}
}
