package biz

import (
	"context"
	stderrors "errors"
	"testing"
)

type fakeTodoRepo struct {
	result  *Todo
	err     error
	created *Todo
	updated *Todo
	deleted int64
}

func (r *fakeTodoRepo) FindByID(context.Context, int64) (*Todo, error) {
	return r.result, r.err
}

func (r *fakeTodoRepo) ListTodos(context.Context, ...ListOption) ([]*Todo, error) {
	return []*Todo{r.result}, r.err
}

func (r *fakeTodoRepo) CreateTodo(_ context.Context, todo *Todo) (*Todo, error) {
	r.created = todo
	return r.result, r.err
}

func (r *fakeTodoRepo) UpdateTodo(_ context.Context, todo *Todo) (*Todo, error) {
	r.updated = todo
	return r.result, r.err
}

func (r *fakeTodoRepo) DeleteTodo(_ context.Context, id int64) error {
	r.deleted = id
	return r.err
}

func TestTodoUsecaseCreateTodo(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "creates valid todo", run: testTodoUsecaseCreatesTodo},
		{name: "validates mutations", run: testTodoUsecaseValidation},
		{name: "returns repo error", run: testTodoUsecaseRepoError},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func testTodoUsecaseCreatesTodo(t *testing.T) {
	want := &Todo{ID: 1, Title: "write tests"}
	repo := &fakeTodoRepo{result: want}
	input := &Todo{Title: "write tests"}

	got, err := NewTodoUsecase(repo).CreateTodo(context.Background(), input)
	if err != nil || got != want {
		t.Fatalf("CreateTodo() = (%v, %v), want (%v, nil)", got, err, want)
	}
	if repo.created != input {
		t.Fatal("CreateTodo() did not pass input to repo")
	}
}

func testTodoUsecaseValidation(t *testing.T) {
	tests := []struct {
		name string
		run  func(*TodoUsecase) error
	}{
		{name: "create nil", run: func(uc *TodoUsecase) error { _, err := uc.CreateTodo(context.Background(), nil); return err }},
		{name: "create empty title", run: func(uc *TodoUsecase) error { _, err := uc.CreateTodo(context.Background(), &Todo{}); return err }},
		{name: "update missing id", run: func(uc *TodoUsecase) error {
			_, err := uc.UpdateTodo(context.Background(), &Todo{Title: "title"})
			return err
		}},
		{name: "update empty title", run: func(uc *TodoUsecase) error { _, err := uc.UpdateTodo(context.Background(), &Todo{ID: 1}); return err }},
		{name: "delete invalid id", run: func(uc *TodoUsecase) error { return uc.DeleteTodo(context.Background(), 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeTodoRepo{}
			if err := tt.run(NewTodoUsecase(repo)); !stderrors.Is(err, ErrTodoInvalidArgument) {
				t.Fatalf("operation error = %v, want %v", err, ErrTodoInvalidArgument)
			}
			if repo.created != nil || repo.updated != nil || repo.deleted != 0 {
				t.Fatal("invalid operation called repo")
			}
		})
	}
}

func testTodoUsecaseRepoError(t *testing.T) {
	repoErr := stderrors.New("repo failed")
	repo := &fakeTodoRepo{err: repoErr}
	_, err := NewTodoUsecase(repo).CreateTodo(context.Background(), &Todo{Title: "title"})
	if !stderrors.Is(err, repoErr) {
		t.Fatalf("CreateTodo() error = %v, want %v", err, repoErr)
	}
}
