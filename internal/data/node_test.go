package data

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sotatts.online/matrix/matrix/packages/backend/go/httpclient"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/testsuite"
)

type heartbeatRecorder struct {
	mu      sync.Mutex
	details []string
}

func (r *heartbeatRecorder) Record(_ *activity.Info, values converter.EncodedValues) {
	var detail string
	if err := values.Get(&detail); err != nil {
		panic(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.details = append(r.details, detail)
}

func (r *heartbeatRecorder) Details() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.details...)
}

func TestWaitTask(t *testing.T) {
	tests := []struct {
		name       string
		taskID     string
		responses  []string
		cancel     bool
		wantOutput map[string]any
		wantError  string
	}{
		{
			name:   "records heartbeat and returns output",
			taskID: "task-1",
			responses: []string{
				"{\"task_id\":\"task-1\",\"status\":\"PROCESSING\"}",
				"{\"task_id\":\"task-1\",\"status\":\"SUCCESS\",\"output\":{\"result\":\"done\"}}",
			},
			wantOutput: map[string]any{"result": "done"},
		},
		{
			name:   "records heartbeat before task failure",
			taskID: "task-2",
			responses: []string{
				"{\"task_id\":\"task-2\",\"status\":\"PENDING\"}",
				"{\"task_id\":\"task-2\",\"status\":\"FAILED\",\"error\":{\"message\":\"model failed\"}}",
			},
			wantError: "MaasTaskFailed",
		},
		{
			name:      "returns when context is canceled",
			taskID:    "task-3",
			responses: []string{"{\"task_id\":\"task-3\",\"status\":\"PROCESSING\"}"},
			cancel:    true,
			wantError: context.Canceled.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := newTaskServer(t, func() string {
				index := int(calls.Add(1)) - 1
				if index >= len(tt.responses) {
					index = len(tt.responses) - 1
				}
				return tt.responses[index]
			})
			defer server.Close()

			client := newTestMaasClient(server)
			recorder := &heartbeatRecorder{}
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestActivityEnvironment()
			env.SetTestTimeout(5 * time.Second)
			env.SetOnActivityHeartbeatListener(recorder.Record)
			activityFn := func(ctx context.Context, taskID string) (map[string]any, error) {
				if tt.cancel {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					time.AfterFunc(50*time.Millisecond, cancel)
				}
				return waitTask(ctx, client, taskID)
			}
			env.RegisterActivity(activityFn)

			value, err := env.ExecuteActivity(activityFn, tt.taskID)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("waitTask activity error = %v, want containing %q", err, tt.wantError)
				}
			} else {
				if err != nil {
					t.Fatalf("waitTask activity error = %v", err)
				}
				var output map[string]any
				if err := value.Get(&output); err != nil {
					t.Fatalf("decode activity output: %v", err)
				}
				if output["result"] != tt.wantOutput["result"] {
					t.Fatalf("waitTask output = %#v, want %#v", output, tt.wantOutput)
				}
			}
			assertHeartbeat(t, recorder.Details(), tt.taskID)
		})
	}
}

func newTaskServer(t *testing.T, response func() string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, modelTaskPath) {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, response()); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
}

func newTestMaasClient(server *httptest.Server) *MaasClient {
	client := NewMaasClient(httpclient.New(server.URL, server.Client(), "maas-test"))
	return &client
}

func assertHeartbeat(t *testing.T, details []string, taskID string) {
	t.Helper()
	if len(details) == 0 {
		t.Fatal("activity did not record a heartbeat")
	}
	for _, detail := range details {
		if detail == taskID {
			return
		}
	}
	t.Fatalf("heartbeat details = %v, want task ID %q", details, taskID)
}
