package task_test

import (
	"encoding/json"
	"log/slog"
	"reflect"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestClient_NewIPDeleteTask(t *testing.T) {
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})
	c := task.NewClient(log, rc)

	type args struct {
		allocationUUID string
		ip             string
		project        string
	}
	tests := []struct {
		name    string
		args    args
		want    any
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				allocationUUID: "a-uuid",
				ip:             "1.2.3.4",
				project:        "project-a",
			},
			want:    task.IPDeletePayload{AllocationUUID: "a-uuid", IP: "1.2.3.4", Project: "project-a"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := c.NewIPDeleteTask(tt.args.allocationUUID, tt.args.ip, tt.args.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("Client.NewIPDeleteTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var payload task.IPDeletePayload
			err = json.Unmarshal(got.Payload, &payload)
			require.NoError(t, err)
			if !reflect.DeepEqual(payload, tt.want) {
				t.Errorf("Client.NewIPDeleteTask() = %v, want %v", payload, tt.want)
			}
		})
	}
}

func TestClient_NewNetworkDeleteTask(t *testing.T) {
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})
	c := task.NewClient(log, rc)

	type args struct {
		uuid string
	}
	tests := []struct {
		name    string
		args    args
		want    task.NetworkDeletePayload
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				uuid: "network-uuid",
			},
			want: task.NetworkDeletePayload{UUID: "network-uuid"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.NewNetworkDeleteTask(tt.args.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("Client.NewNetworkDeleteTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var payload task.NetworkDeletePayload
			err = json.Unmarshal(got.Payload, &payload)
			require.NoError(t, err)
			if !reflect.DeepEqual(payload, tt.want) {
				t.Errorf("Client.NewNetworkDeleteTask() = %v, want %v", payload, tt.want)
			}
		})
	}
}

func TestClient_NewMachineDeleteTask(t *testing.T) {
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})
	c := task.NewClient(log, rc)

	type args struct {
		uuid           *string
		allocationUUID *string
	}
	tests := []struct {
		name    string
		args    args
		want    task.MachineDeletePayload
		wantErr bool
	}{
		{
			name: "simple with machine uuid",
			args: args{
				uuid: pointer.Pointer("machine-uuid"),
			},
			want: task.MachineDeletePayload{UUID: pointer.Pointer("machine-uuid")},
		},
		{
			name: "simple with allocation uuid",
			args: args{
				allocationUUID: pointer.Pointer("allocation-uuid"),
			},
			want: task.MachineDeletePayload{AllocationUUID: pointer.Pointer("allocation-uuid")},
		},
		{
			name: "simple with allocation and machine uuid",
			args: args{
				uuid:           pointer.Pointer("machine-uuid"),
				allocationUUID: pointer.Pointer("allocation-uuid"),
			},
			want: task.MachineDeletePayload{UUID: pointer.Pointer("machine-uuid"), AllocationUUID: pointer.Pointer("allocation-uuid")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.NewMachineDeleteTask(tt.args.uuid, tt.args.allocationUUID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Client.NewMachineDeleteTask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var payload task.MachineDeletePayload
			err = json.Unmarshal(got.Payload, &payload)
			require.NoError(t, err)
			if !reflect.DeepEqual(payload, tt.want) {
				t.Errorf("Client.NewMachineDeleteTask() = %v, want %v", payload, tt.want)
			}
		})
	}
}

func TestClient_Informers(t *testing.T) {
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})
	c := task.NewClient(log, rc)

	task, err := c.NewMachineDeleteTask(pointer.Pointer("machine-uuid"), pointer.Pointer("allocation-uuid"))
	require.NoError(t, err)

	qs, err := c.GetQueues()
	require.NoError(t, err)
	require.NotEmpty(t, qs)

	taskInfo, err := c.GetTaskInfo("default", task.ID)
	require.NoError(t, err)
	require.NotNil(t, taskInfo)

	taskList, err := c.List(nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, taskList, 1)
	require.Equal(t, taskInfo.ID, taskList[0].ID)

}
