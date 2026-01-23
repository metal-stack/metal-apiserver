package admin

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type taskServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.TaskServiceHandler {
	return &taskServiceServer{
		log:  c.Log.WithGroup("taskService"),
		repo: c.Repo,
	}
}

func (t *taskServiceServer) Get(_ context.Context, req *adminv2.TaskServiceGetRequest) (*adminv2.TaskServiceGetResponse, error) {
	taskInfo, err := t.repo.Task().GetTaskInfo(req.Queue, req.TaskId)
	if err != nil {
		return nil, err
	}
	return &adminv2.TaskServiceGetResponse{
		Task: toProto(taskInfo),
	}, nil
}

func (t *taskServiceServer) ListTasks(_ context.Context, req *adminv2.TaskServiceListTasksRequest) (*adminv2.TaskServiceListTasksResponse, error) {
	taskList, err := t.repo.Task().ListTasks(req.Queue, req.Count, req.Page)
	if err != nil {
		return nil, err
	}
	return &adminv2.TaskServiceListTasksResponse{
		TaskList: toTaskList(taskList),
	}, nil
}

func (t *taskServiceServer) Queues(_ context.Context, req *adminv2.TaskServiceQueuesRequest) (*adminv2.TaskServiceQueuesResponse, error) {
	queues, err := t.repo.Task().GetQueues()
	if err != nil {
		return nil, err
	}
	return &adminv2.TaskServiceQueuesResponse{
		Queues: queues,
	}, nil
}

func toTaskList(tl *task.TaskList) *adminv2.TaskList {
	taskList := &adminv2.TaskList{
		Active:      toProtos(tl.Active),
		Aggregating: toProtos(tl.Aggregating),
		Archived:    toProtos(tl.Archived),
		Completed:   toProtos(tl.Completed),
		Pending:     toProtos(tl.Pending),
		Retry:       toProtos(tl.Retry),
		Scheduled:   toProtos(tl.Scheduled),
	}

	return taskList
}

func toProtos(ts []*asynq.TaskInfo) []*adminv2.TaskInfo {
	var tasks []*adminv2.TaskInfo
	for _, t := range ts {
		tasks = append(tasks, toProto(t))
	}
	return tasks
}

func toProto(t *asynq.TaskInfo) *adminv2.TaskInfo {
	var state adminv2.TaskState
	switch t.State {
	case asynq.TaskStateActive:
		state = adminv2.TaskState_TASK_STATE_ACTIVE
	case asynq.TaskStateAggregating:
		state = adminv2.TaskState_TASK_STATE_AGGREGATING
	case asynq.TaskStateArchived:
		state = adminv2.TaskState_TASK_STATE_ARCHIVED
	case asynq.TaskStateCompleted:
		state = adminv2.TaskState_TASK_STATE_COMPLETED
	case asynq.TaskStatePending:
		state = adminv2.TaskState_TASK_STATE_PENDING
	case asynq.TaskStateRetry:
		state = adminv2.TaskState_TASK_STATE_RETRY
	case asynq.TaskStateScheduled:
		state = adminv2.TaskState_TASK_STATE_SCHEDULED
	}

	result := &adminv2.TaskInfo{
		Id:            t.ID,
		Queue:         t.Queue,
		Type:          t.Type,
		Payload:       t.Payload,
		State:         state,
		MaxRetry:      int32(t.MaxRetry),
		Retried:       int32(t.Retried),
		LastError:     t.LastErr,
		LastFailedAt:  timestamppb.New(t.LastFailedAt),
		Timeout:       durationpb.New(t.Timeout),
		Deadline:      timestamppb.New(t.Deadline),
		Group:         t.Group,
		NextProcessAt: timestamppb.New(t.NextProcessAt),
		IsOrphaned:    t.IsOrphaned,
		Retention:     durationpb.New(t.Retention),
		CompletedAt:   timestamppb.New(t.CompletedAt),
		Result:        t.Result,
	}

	return result
}
