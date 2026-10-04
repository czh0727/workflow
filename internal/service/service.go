package service

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewTodoService, NewWorkflowService, NewWorkerService)
