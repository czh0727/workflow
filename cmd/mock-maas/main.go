// mock-maas 是本地学习和测试使用的 MaaS API Gateway 替身。
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type task struct {
	id      string
	path    string
	request map[string]any
	polls   int
	created time.Time
	output  map[string]any
	failed  bool
}

type server struct {
	mu              sync.Mutex
	tasks           map[string]*task
	sequence        uint64
	processingPolls int
	mode            string
}

func main() {
	addr := envOr("MOCK_MAAS_ADDR", ":18081")
	processingPolls := envInt("MOCK_MAAS_PROCESSING_POLLS", 0)
	mode := strings.ToLower(strings.TrimSpace(envOr("MOCK_MAAS_MODE", "success")))

	s := &server{
		tasks:           make(map[string]*task),
		processingPolls: processingPolls,
		mode:            mode,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/internal/v2/tasks/", s.getTask)
	mux.HandleFunc("/internal/v2/", s.submit)

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("MaaS 本地 Mock 已启动", "addr", addr, "mode", mode, "processing_polls", processingPolls)
	if err := http.ListenAndServe(addr, loggingMiddleware(log, mux)); err != nil {
		log.Error("MaaS 本地 Mock 退出", "error", err)
		os.Exit(1)
	}
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *server) submit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error_code": 400,
			"error_msg":  "请求体不是有效 JSON",
		})
		return
	}

	id := fmt.Sprintf("mock-task-%d", atomic.AddUint64(&s.sequence, 1))
	s.mu.Lock()
	s.tasks[id] = &task{
		id:      id,
		path:    r.URL.Path,
		request: request,
		created: time.Now(),
		output:  outputFor(r.URL.Path, id, request),
		failed:  s.mode == "failed",
	}
	s.mu.Unlock()

	status := "PENDING"
	if s.processingPolls == 0 && s.mode != "failed" {
		status = "SUCCESS"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"error_code":          0,
		"error_msg":           "",
		"internal_error_code": 0,
		"internal_error_msg":  "",
		"data": map[string]any{
			"object":         "task",
			"client_task_id": id,
			"task_id":        id,
			"status":         status,
			"created_at":     time.Now().Unix(),
			"updated_at":     time.Now().Unix(),
		},
	})
}

func (s *server) getTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/internal/v2/tasks/")
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "task not found"})
		return
	}
	t.polls++
	status := "PROCESSING"
	if t.failed {
		status = "FAILED"
	} else if t.polls > s.processingPolls {
		status = "SUCCESS"
	}

	response := map[string]any{
		"task_id":    t.id,
		"object":     "task",
		"type":       strings.TrimPrefix(t.path, "/internal/v2/"),
		"status":     status,
		"created_at": t.created.UTC().Format(time.RFC3339),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if status == "SUCCESS" {
		response["output"] = t.output
	}
	if status == "FAILED" {
		response["error"] = map[string]any{"message": "MaaS Mock 按配置返回失败"}
	}
	writeJSON(w, http.StatusOK, response)
}

func outputFor(path, id string, request map[string]any) map[string]any {
	base := "http://127.0.0.1:18081/assets/" + id
	switch {
	case strings.Contains(path, "image_generation"):
		return map[string]any{"kind": "image", "image_url": base + ".png", "result_asset_id": id}
	case strings.Contains(path, "video_generation"):
		return map[string]any{"kind": "video", "video_url": base + ".mp4", "result_asset_id": id}
	case strings.Contains(path, "audio_transcription"), strings.Contains(path, "transcription_diarization"):
		text := "这是 MaaS Mock 生成的转写文本"
		if value, ok := request["text"].(string); ok && value != "" {
			text = value
		}
		return map[string]any{"kind": "transcription", "text": text, "segments": []map[string]any{{"text": text, "start_time": 0, "end_time": 1}}}
	default:
		return map[string]any{"kind": "audio", "audio_url": base + ".mp3", "audio_asset_id": id, "duration_ms": 1000}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Info("MaaS Mock 请求", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(envOr(name, strconv.Itoa(fallback)))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
