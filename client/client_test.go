package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	testCases := []struct {
		args        string
		shouldError bool
		expected    clientState
	}{
		// Valid put
		{
			args:        "client put blah:1234 somefile.txt",
			shouldError: false,
			expected: clientState{
				mode:     modePut,
				filename: "somefile.txt",
				host:     "blah",
				port:     "1234",
			},
		},
		{
			args:        "client PUT blah:1234 somefile.txt",
			shouldError: false,
			expected: clientState{
				mode:     modePut,
				filename: "somefile.txt",
				host:     "blah",
				port:     "1234",
			},
		},
		// Valid get
		{
			args:        "client get blah:1234 somefile.txt",
			shouldError: false,
			expected: clientState{
				mode:     modeGet,
				filename: "somefile.txt",
				host:     "blah",
				port:     "1234",
			},
		},
		{
			args:        "client GET blah:1234 somefile.txt",
			shouldError: false,
			expected: clientState{
				mode:     modeGet,
				filename: "somefile.txt",
				host:     "blah",
				port:     "1234",
			},
		},
		// Not enough args
		{
			args:        "client get blah:1234",
			shouldError: true,
			expected:    clientState{},
		},
		// Unknown command
		{
			args:        "client abc blah:1234 somefile.txt",
			shouldError: true,
			expected:    clientState{},
		},
	}

	for i, tc := range testCases {
		args := strings.Fields(tc.args)
		cs, err := parseArgs(args)
		if tc.shouldError && err == nil {
			t.Errorf("Expected an error, didn't get one (%d)", i)
			continue
		}
		if !tc.shouldError && err != nil {
			t.Errorf("Didn't expect an error: %v (%d)", err, i)
			continue
		}
		if !reflect.DeepEqual(cs, tc.expected) {
			t.Errorf("Case %d failed", i)
			t.Error("Got")
			t.Errorf("%+v", cs)
			t.Error("Expected")
			t.Errorf("%+v", tc.expected)
		}
	}
}
