package service

import (
	"context"

	pb "git.sotatts.online/matrix/matrix/workflow/api-server/api/worker/v1"
	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/biz"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

// WorkerService 提供 Worker 服务。
type WorkerService struct {
	pb.UnimplementedWorkerServiceServer

	worker *biz.WorkerUsecase
}

// NewWorkerService 创建 Worker 服务。
func NewWorkerService(worker *biz.WorkerUsecase) *WorkerService {
	return &WorkerService{worker: worker}
}

// RegisterTemporalWorker 注册 WorkerService 提供的 Workflow 和 Activity。
func (s *WorkerService) RegisterTemporalWorker(w worker.Worker) {
	w.RegisterWorkflowWithOptions(s.worker.DynamicWorkflow, temporalworkflow.RegisterOptions{
		Name: biz.DynamicWorkflowType,
	})
	w.RegisterActivityWithOptions(s.worker.UpdateExecutionStatus, activity.RegisterOptions{
		Name: biz.UpdateExecutionStatusActivityType,
	})
	for _, node := range s.worker.GetActivityNodes() {
		w.RegisterActivityWithOptions(node.Activity, activity.RegisterOptions{
			Name: string(node.Type()),
		})
	}
}

func (*WorkerService) Healthz(context.Context, *pb.HealthzRequest) (*pb.HealthzResponse, error) {
	return &pb.HealthzResponse{}, nil
}

func (*WorkerService) Readyz(context.Context, *pb.ReadyzRequest) (*pb.ReadyzResponse, error) {
	return &pb.ReadyzResponse{}, nil
}
