//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

// Contains general utility code.
package util

import "testing"

func TestLogger_Log(t *testing.T) {
	type fields struct {
		Level uint
	}
	type args struct {
		debugLevel uint
		format     string
		v          []interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "Testcase 1",
			fields: fields{
				Level: 0,
			},
			args: args{
				debugLevel: 0,
				format:     "Test",
				v:          []interface{}{},
			},
		},
		{
			name: "Testcase 2",
			fields: fields{
				Level: 1,
			},
			args: args{
				debugLevel: 1,
				format:     "Test",
				v:          []interface{}{},
			},
		},
		{
			name: "Testcase 3",
			fields: fields{
				Level: 2,
			},
			args: args{
				debugLevel: 2,
				format:     "Test",
				v:          []interface{}{},
			},
		},
		{
			name: "Testcase 4",
			fields: fields{
				Level: 3,
			},
			args: args{
				debugLevel: 3,
				format:     "Test",
				v:          []interface{}{},
			},
		},
		{
			name: "Testcase 5",
			fields: fields{
				Level: 4,
			},
			args: args{
				debugLevel: 4,
				format:     "Test",
				v:          []interface{}{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lg := &Logger{
				Level: tt.fields.Level,
			}
			lg.Log(tt.args.debugLevel, tt.args.format, tt.args.v...)
		})
	}
}
