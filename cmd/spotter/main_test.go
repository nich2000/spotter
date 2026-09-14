package main

import (
	"flag"
	"io"
	"testing"
)

func TestPortFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    int
		invalid bool
	}{
		{"config default", nil, 0, false},
		{"short", []string{"-p", "8081"}, 8081, false},
		{"long", []string{"--port", "8082"}, 8082, false},
		{"equals", []string{"--port=65535"}, 65535, false},
		{"last wins", []string{"-p", "8081", "--port", "8082"}, 8082, false},
		{"zero", []string{"-p", "0"}, 0, true},
		{"negative", []string{"--port=-1"}, 0, true},
		{"too large", []string{"-p", "65536"}, 0, true},
		{"not integer", []string{"--port", "abc"}, 0, true},
		{"missing", []string{"-p"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet("spotter", flag.ContinueOnError)
			flags.SetOutput(io.Discard)
			var port int
			registerPortFlags(flags, &port)
			err := flags.Parse(tc.args)
			if (err != nil) != tc.invalid || port != tc.want {
				t.Fatalf("port=%d err=%v, want port=%d invalid=%v", port, err, tc.want, tc.invalid)
			}
		})
	}
}
