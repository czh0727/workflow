package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/biz"

	"github.com/go-viper/mapstructure/v2"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

type nodeRepo struct {
	nodes map[biz.NodeType]biz.ActivityNode
}

var (
	_ biz.NodeRepo       = (*nodeRepo)(nil)
	_ biz.WorkerNodeRepo = (*nodeRepo)(nil)
)

// NewNodeRepo 创建节点信息仓库。
func NewNodeRepo(data *Data) biz.NodeRepo {
	return newNodeRepo(data, nil)
}

// NewWorkerNodeRepo 创建 Worker 节点仓库。
func NewWorkerNodeRepo(data *Data, maasClient MaasClient) biz.WorkerNodeRepo {
	return newNodeRepo(data, &maasClient)
}

func newNodeRepo(data *Data, maasClient *MaasClient) *nodeRepo {
	saySomethingNode := &saySomethingNode{}
	exampleNode := &exampleNode{client: data.httpClient}
	imageGenerationNode := &imageGenerationNode{client: maasClient}
	videoGenerationNode := &videoGenerationNode{client: maasClient}
	ttsNode := &ttsNode{client: maasClient}
	ttsdNode := &ttsdNode{client: maasClient}
	voiceGeneratorNode := &voiceGeneratorNode{client: maasClient}
	voiceConvertNode := &voiceConvertNode{client: maasClient}
	speechEnhanceNode := &speechEnhanceNode{client: maasClient}
	soundEffectGenerationNode := &soundEffectGenerationNode{client: maasClient}
	audioTranscriptionNode := &audioTranscriptionNode{client: maasClient}
	transcriptionDiarizationNode := &transcriptionDiarizationNode{client: maasClient}
	return &nodeRepo{
		nodes: map[biz.NodeType]biz.ActivityNode{
			saySomethingNode.Type():             saySomethingNode,
			exampleNode.Type():                  exampleNode,
			imageGenerationNode.Type():          imageGenerationNode,
			videoGenerationNode.Type():          videoGenerationNode,
			ttsNode.Type():                      ttsNode,
			ttsdNode.Type():                     ttsdNode,
			voiceGeneratorNode.Type():           voiceGeneratorNode,
			voiceConvertNode.Type():             voiceConvertNode,
			speechEnhanceNode.Type():            speechEnhanceNode,
			soundEffectGenerationNode.Type():    soundEffectGenerationNode,
			audioTranscriptionNode.Type():       audioTranscriptionNode,
			transcriptionDiarizationNode.Type(): transcriptionDiarizationNode,
		},
	}
}

func (r *nodeRepo) GetActivityNode(nodeType biz.NodeType) biz.ActivityNode {
	return r.nodes[nodeType]
}

func (r *nodeRepo) GetActivityNodes() []biz.ActivityNode {
	nodeTypes := slices.Sorted(maps.Keys(r.nodes))
	nodes := make([]biz.ActivityNode, 0, len(nodeTypes))
	for _, nodeType := range nodeTypes {
		nodes = append(nodes, r.nodes[nodeType])
	}
	return nodes
}

type saySomethingNode struct{}

type saySomethingInput struct {
	Prompt *string `mapstructure:"prompt"`
}

func (*saySomethingNode) Type() biz.NodeType {
	return biz.NodeTypeSaySomething
}

func (*saySomethingNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type: biz.NodeTypeSaySomething,
		Inputs: map[string]biz.InputDefinition{
			"prompt": {
				Type:     "string",
				Required: true,
			},
		},
		Outputs: map[string]biz.OutputDefinition{
			"text": {Type: "string"},
		},
	}
}

func (*saySomethingNode) Activity(_ context.Context, inputValues map[string]any) (map[string]any, error) {
	var input saySomethingInput
	if err := mapstructure.Decode(inputValues, &input); err != nil {
		return nil, temporal.NewNonRetryableApplicationError(
			"decode say something input",
			"SaySomethingInvalidInput",
			err,
		)
	}
	if input.Prompt == nil {
		return nil, temporal.NewNonRetryableApplicationError(
			"say something prompt must be a string",
			"SaySomethingInvalidInput",
			biz.ErrDynamicWorkflowInvalidDefinition,
		)
	}
	return map[string]any{"text": *input.Prompt}, nil
}

type exampleNode struct {
	client *http.Client
}

const exampleURL = "https://example.com"

func (*exampleNode) Type() biz.NodeType {
	return biz.NodeTypeExample
}

func (*exampleNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:   biz.NodeTypeExample,
		Inputs: map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{
			"status_code": {Type: "integer"},
			"body":        {Type: "string"},
		},
	}
}

func (n *exampleNode) Activity(ctx context.Context, _ map[string]any) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, exampleURL, nil)
	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(
			"create example request",
			"ExampleRequestInvalid",
			err,
		)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("example request failed with status %s", resp.Status)
	}
	return map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(body),
	}, nil
}

const modelTaskPollInterval = 2 * time.Second

type imageGenerationNode struct{ client *MaasClient }

func (*imageGenerationNode) Type() biz.NodeType { return biz.NodeTypeImageGeneration }
func (n *imageGenerationNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *imageGenerationNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req ImageGenerationRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.ImageGeneration(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type videoGenerationNode struct{ client *MaasClient }

func (*videoGenerationNode) Type() biz.NodeType { return biz.NodeTypeVideoGeneration }
func (n *videoGenerationNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *videoGenerationNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req VideoGenerationRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.VideoGeneration(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type ttsNode struct{ client *MaasClient }

func (*ttsNode) Type() biz.NodeType { return biz.NodeTypeTTS }
func (n *ttsNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *ttsNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req TTSRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.TTS(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type ttsdNode struct{ client *MaasClient }

func (*ttsdNode) Type() biz.NodeType { return biz.NodeTypeTTSD }
func (n *ttsdNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *ttsdNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req TTSDRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.TTSD(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type voiceGeneratorNode struct{ client *MaasClient }

func (*voiceGeneratorNode) Type() biz.NodeType { return biz.NodeTypeVoiceGenerator }
func (n *voiceGeneratorNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *voiceGeneratorNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req VoiceGeneratorRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.VoiceGenerator(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type voiceConvertNode struct{ client *MaasClient }

func (*voiceConvertNode) Type() biz.NodeType { return biz.NodeTypeVoiceConvert }
func (n *voiceConvertNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *voiceConvertNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req VoiceConvertRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.VoiceConvert(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type speechEnhanceNode struct{ client *MaasClient }

func (*speechEnhanceNode) Type() biz.NodeType { return biz.NodeTypeSpeechEnhance }
func (n *speechEnhanceNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *speechEnhanceNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req SpeechEnhanceRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.SpeechEnhance(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type soundEffectGenerationNode struct{ client *MaasClient }

func (*soundEffectGenerationNode) Type() biz.NodeType { return biz.NodeTypeSoundEffectGeneration }
func (n *soundEffectGenerationNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *soundEffectGenerationNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req SoundEffectGenerationRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.SoundEffectGeneration(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type audioTranscriptionNode struct{ client *MaasClient }

func (*audioTranscriptionNode) Type() biz.NodeType { return biz.NodeTypeAudioTranscription }
func (n *audioTranscriptionNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *audioTranscriptionNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req AudioTranscriptionRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.AudioTranscription(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

type transcriptionDiarizationNode struct{ client *MaasClient }

func (*transcriptionDiarizationNode) Type() biz.NodeType {
	return biz.NodeTypeTranscriptionDiarization
}
func (n *transcriptionDiarizationNode) Schema() biz.NodeSchema {
	return biz.NodeSchema{
		Type:    n.Type(),
		Inputs:  map[string]biz.InputDefinition{},
		Outputs: map[string]biz.OutputDefinition{},
	}
}
func (n *transcriptionDiarizationNode) Activity(ctx context.Context, input map[string]any) (map[string]any, error) {
	var req TranscriptionDiarizationRequest
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &req,
		TagName: "json",
		Squash:  true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(input); err != nil {
		return nil, err
	}
	task, err := n.client.TranscriptionDiarization(ctx, req)
	if err != nil {
		return nil, err
	}
	return waitTask(ctx, n.client, task.TaskID)
}

func waitTask(ctx context.Context, client *MaasClient, taskID string) (map[string]any, error) {
	if taskID == "" {
		return nil, temporal.NewNonRetryableApplicationError(
			"maas response has no task id",
			"MaasInvalidResponse",
			nil,
		)
	}
	logger := activity.GetLogger(ctx)
	logger.Info(
		"maas task submitted",
		"task_id", taskID,
	)
	startedAt := time.Now()

	for {
		task, err := client.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		status := strings.ToUpper(strings.TrimSpace(task.Status))
		switch status {
		case "SUCCESS", "COMPLETED":
			logger.Info(
				"maas task completed",
				"task_id", taskID,
				"status", status,
				"duration_ms", time.Since(startedAt).Seconds()*1000,
			)
			if len(task.Output) == 0 || string(task.Output) == "null" {
				return map[string]any{}, nil
			}
			var output map[string]any
			if err := json.Unmarshal(task.Output, &output); err != nil {
				return nil, fmt.Errorf("decode maas task output: %w", err)
			}
			return output, nil
		case "FAILED", "CANCELLED":
			logger.Info(
				"maas task completed",
				"task_id", taskID,
				"status", status,
				"duration_ms", time.Since(startedAt).Seconds()*1000,
			)
			message := fmt.Sprintf("maas task %s ended with status %s", taskID, task.Status)
			if len(task.Error) > 0 && string(task.Error) != "null" {
				message += ": " + string(task.Error)
			}
			return nil, temporal.NewNonRetryableApplicationError(message, "MaasTaskFailed", nil)
		case "INIT", "PENDING", "PROCESSING", "CANCELLING":
		default:
			return nil, temporal.NewNonRetryableApplicationError(
				fmt.Sprintf("maas task %s has unknown status %q", taskID, task.Status),
				"MaasInvalidTaskStatus",
				nil,
			)
		}

		activity.RecordHeartbeat(ctx, taskID)
		timer := time.NewTimer(modelTaskPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
