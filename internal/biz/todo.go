package biz

import (
	"context"
	"strings"
	"time"

	v1 "git.sotatts.online/matrix/matrix/workflow/api-server/api/todo/v1"

	"github.com/go-kratos/kratos/v3/errors"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
)

var (
	// ErrTodoNotFound 表示待办事项不存在。
	ErrTodoNotFound = errors.NotFound(v1.ErrorReason_TODO_NOT_FOUND.String(), "todo not found")
	// ErrTodoInvalidArgument 表示待办事项请求参数无效。
	ErrTodoInvalidArgument = errors.BadRequest(v1.ErrorReason_TODO_INVALID_ARGUMENT.String(), "invalid todo argument")
)

// Todo 是待办事项领域模型。
type Todo struct {
	ID         int64
	Title      string
	Content    string
	Completed  bool
	CreateTime time.Time
	UpdateTime time.Time
}

// TodoRepo 定义待办事项仓储接口。
type TodoRepo interface {
	FindByID(context.Context, int64) (*Todo, error)
	ListTodos(context.Context, ...ListOption) ([]*Todo, error)
	CreateTodo(context.Context, *Todo) (*Todo, error)
	UpdateTodo(context.Context, *Todo) (*Todo, error)
	DeleteTodo(context.Context, int64) error
}

// ListOption 配置待办事项列表查询。
type ListOption func(*ListOptions)

// ListOptions 是待办事项列表查询选项。
type ListOptions struct {
	Filter  filtering.Filter
	OrderBy ordering.OrderBy
	Offset  int
	Limit   int
}

// ListFilter 设置标准 AIP 过滤条件。
func ListFilter(filter filtering.Filter) ListOption {
	return func(o *ListOptions) {
		o.Filter = filter
	}
}

// ListOrderBy 设置标准 AIP order_by 值。
func ListOrderBy(orderBy ordering.OrderBy) ListOption {
	return func(o *ListOptions) {
		o.OrderBy = orderBy
	}
}

// ListOffset 设置偏移量。
func ListOffset(offset int) ListOption {
	return func(o *ListOptions) {
		o.Offset = offset
	}
}

// ListLimit 设置返回数量上限。
func ListLimit(limit int) ListOption {
	return func(o *ListOptions) {
		o.Limit = limit
	}
}

// TodoUsecase 负责待办事项业务逻辑。
type TodoUsecase struct {
	repo TodoRepo
}

// NewTodoUsecase 创建待办事项业务用例。
func NewTodoUsecase(repo TodoRepo) *TodoUsecase {
	return &TodoUsecase{repo: repo}
}

// GetTodo 根据 ID 返回待办事项。
func (uc *TodoUsecase) GetTodo(ctx context.Context, id int64) (*Todo, error) {
	return uc.repo.FindByID(ctx, id)
}

// ListTodos 查询待办事项列表。
func (uc *TodoUsecase) ListTodos(ctx context.Context, opts ...ListOption) ([]*Todo, error) {
	return uc.repo.ListTodos(ctx, opts...)
}

// CreateTodo 创建待办事项。
func (uc *TodoUsecase) CreateTodo(ctx context.Context, todo *Todo) (*Todo, error) {
	if err := validateTodo(todo); err != nil {
		return nil, err
	}
	return uc.repo.CreateTodo(ctx, todo)
}

// UpdateTodo 更新待办事项。
func (uc *TodoUsecase) UpdateTodo(ctx context.Context, todo *Todo) (*Todo, error) {
	if todo == nil || todo.ID <= 0 {
		return nil, ErrTodoInvalidArgument
	}
	if err := validateTodo(todo); err != nil {
		return nil, err
	}
	return uc.repo.UpdateTodo(ctx, todo)
}

// DeleteTodo 删除待办事项。
func (uc *TodoUsecase) DeleteTodo(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrTodoInvalidArgument
	}
	return uc.repo.DeleteTodo(ctx, id)
}

func validateTodo(todo *Todo) error {
	if todo == nil || strings.TrimSpace(todo.Title) == "" {
		return ErrTodoInvalidArgument
	}
	return nil
}
