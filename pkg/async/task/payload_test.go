package task_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hibiken/asynq"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func TestEncodePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload task.TaskPayload
		want    string
		wantErr bool
	}{
		{
			name: "encode payload",
			payload: &task.MachineBMCCommandPayload{
				UUID:      "1",
				Partition: "2",
				Command:   "3",
				CommandID: "4",
			},
			want: `{"uuid":"1","partition":"2","command":"3","command_id":"4"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := task.EncodePayload(tt.payload)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("EncodePayload() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("EncodePayload() succeeded unexpectedly")
			}
			if string(got) != tt.want {
				t.Errorf("EncodePayload() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestDecodePayload(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    *task.MachineBMCCommandPayload
		wantErr error
	}{
		{
			name: "decode payload",
			data: []byte(`{"uuid":"1","partition":"2","command":"3","command_id":"4"}`),
			want: &task.MachineBMCCommandPayload{
				UUID:      "1",
				Partition: "2",
				Command:   "3",
				CommandID: "4",
			},
		},
		{
			name:    "skip retry",
			data:    []byte(`{"uuid":"1","partitio`),
			want:    nil,
			wantErr: fmt.Errorf("unable to unmarshal task payload, %w: unexpected end of JSON input%w", asynq.SkipRetry, &json.SyntaxError{}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := task.DecodePayload[*task.MachineBMCCommandPayload](tt.data)
			if diff := cmp.Diff(tt.wantErr, gotErr, errorutil.ErrorStringComparer(), cmpopts.IgnoreUnexported(json.SyntaxError{})); diff != "" {
				t.Errorf("error diff (+got -want):\n %s", diff)
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("DecodePayload() = %s", diff)
			}
		})
	}
}
