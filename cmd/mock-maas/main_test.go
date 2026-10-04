package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockMaaSTaskLifecycle(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		processingPolls int
		wantSubmit      string
		wantStatuses    []string
	}{
		{name: "立即成功", wantSubmit: "SUCCESS", wantStatuses: []string{"SUCCESS"}},
		{name: "轮询后成功", processingPolls: 2, wantSubmit: "PENDING", wantStatuses: []string{"PROCESSING", "PROCESSING", "SUCCESS"}},
		{name: "失败任务", mode: "failed", wantSubmit: "PENDING", wantStatuses: []string{"FAILED"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &server{
				tasks:           make(map[string]*task),
				processingPolls: test.processingPolls,
				mode:            test.mode,
			}

			// 空请求体应被拒绝，保证 Mock 与真实网关一样校验 JSON。
			empty := httptest.NewRecorder()
			s.submit(empty, httptest.NewRequest(http.MethodPost, "/internal/v2/tts", strings.NewReader("")))
			if empty.Code != http.StatusBadRequest {
				t.Fatalf("空请求体状态码 = %d，期望 %d", empty.Code, http.StatusBadRequest)
			}

			submit := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal/v2/tts", strings.NewReader(`{"text":"hello"}`))
			s.submit(submit, req)
			if submit.Code != http.StatusOK {
				t.Fatalf("提交状态码 = %d，期望 %d", submit.Code, http.StatusOK)
			}
			var envelope struct {
				Data struct {
					TaskID string `json:"task_id"`
					Status string `json:"status"`
				} `json:"data"`
			}
			if err := json.Unmarshal(submit.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Status != test.wantSubmit {
				t.Fatalf("提交状态 = %q，期望 %q", envelope.Data.Status, test.wantSubmit)
			}

			for _, wantStatus := range test.wantStatuses {
				get := httptest.NewRecorder()
				s.getTask(get, httptest.NewRequest(http.MethodGet, "/internal/v2/tasks/"+envelope.Data.TaskID, nil))
				var response struct {
					Status string `json:"status"`
				}
				if err := json.Unmarshal(get.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Status != wantStatus {
					t.Fatalf("任务状态 = %q，期望 %q", response.Status, wantStatus)
				}
			}
		})
	}
}
