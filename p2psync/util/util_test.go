//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

// Contains general utility code.
package util

import "testing"

func TestLog(t *testing.T) {
	type args struct {
		debugLevel int
		format     string
		v          []interface{}
	}
	tests := []struct {
		name string
		args args
	}{
		{"Test1", args{0, "test", nil}},
		{"Test2", args{1, "test", nil}},
		{"Test3", args{2, "test", nil}},
		{"Test4", args{3, "test", nil}},
		{"Test5", args{4, "test", nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Log(tt.args.debugLevel, tt.args.format, tt.args.v...)
		})
	}
}
