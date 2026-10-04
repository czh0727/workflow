package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"git.sotatts.online/matrix/matrix/workflow/api-server/internal/conf"
	"git.sotatts.online/matrix/matrix/workflow/api-server/pkg/request"

	"git.sotatts.online/matrix/matrix/packages/backend/go/httpclient"
	sharedobs "git.sotatts.online/matrix/matrix/packages/backend/go/obs"
)

const (
	maasServiceTokenHeader       = "X-Service-Token"
	imageGenerationPath          = "/internal/v2/image_generation?allow_envelope=true"
	videoGenerationPath          = "/internal/v2/video_generation?allow_envelope=true"
	ttsPath                      = "/internal/v2/tts?allow_envelope=true"
	ttsdPath                     = "/internal/v2/ttsd?allow_envelope=true"
	voiceGeneratorPath           = "/internal/v2/voice_generator?allow_envelope=true"
	voiceConvertPath             = "/internal/v2/voice_convert?allow_envelope=true"
	speechEnhancePath            = "/internal/v2/speech_enhance?allow_envelope=true"
	soundEffectGenerationPath    = "/internal/v2/sound_effect_generation?allow_envelope=true"
	audioTranscriptionPath       = "/internal/v2/audio_transcription?allow_envelope=true"
	transcriptionDiarizationPath = "/internal/v2/transcription_diarization?allow_envelope=true"
	modelTaskPath                = "/internal/v2/tasks/"
)

func NewMaasHTTPClient(c *conf.Data) (*httpclient.Client, error) {
	if c == nil || c.MaasApiGateway == nil || strings.TrimSpace(c.MaasApiGateway.BaseUrl) == "" {
		return nil, errors.New("maas api gateway config is required")
	}

	client := sharedobs.NewHTTPClient()
	if c.MaasApiGateway.Timeout != nil {
		client.Timeout = c.MaasApiGateway.Timeout.AsDuration()
	}

	httpClient := httpclient.New(strings.TrimRight(c.MaasApiGateway.BaseUrl, "/"), client, "maas-api-gateway")
	if token := strings.TrimSpace(c.MaasApiGateway.ServiceToken); token != "" {
		httpClient.DefaultHeaders = map[string]string{maasServiceTokenHeader: token}
	}
	return httpClient, nil
}

type MaasClient struct {
	httpClient *httpclient.Client
}

func NewMaasClient(httpClient *httpclient.Client) MaasClient {
	httpClient.Use(maasRequestContext())
	return MaasClient{
		httpClient: httpClient,
	}
}

func maasRequestContext() httpclient.Middleware {
	return func(next httpclient.HandlerFunc) httpclient.HandlerFunc {
		return func(ctx context.Context, req *httpclient.Request) (*httpclient.Response, error) {
			info, _ := request.FromContext(ctx)
			if req.Header == nil {
				req.Header = make(http.Header)
			}
			req.Header.Set(request.AppIDHeader, info.AppID)
			req.Header.Set(request.OriginIDHeader, info.OriginID)
			req.Header.Set(request.SubjectIDHeader, info.SubjectID)
			req.Header.Set(request.RequestIDHeader, info.RequestID)
			return next(ctx, req)
		}
	}
}

type ModelResponse[T any] struct {
	Object        string          `json:"object"`
	ClientTaskID  string          `json:"client_task_id"`
	TaskID        string          `json:"task_id"`
	Status        string          `json:"status"`
	ModelID       string          `json:"model_id,omitempty"`
	ModelVersion  string          `json:"model_version,omitempty"`
	RetryAfterSec int             `json:"retry_after,omitempty"`
	CreatedAt     int64           `json:"created_at,omitempty"`
	UpdatedAt     int64           `json:"updated_at,omitempty"`
	CompletedAt   int64           `json:"completed_at,omitempty"`
	Error         json.RawMessage `json:"error,omitempty"`
	Result        T               `json:"result,omitempty"`
}

type maasEnvelope[T any] struct {
	ErrorCode         int    `json:"error_code"`
	ErrorMsg          string `json:"error_msg"`
	InternalErrorCode int    `json:"internal_error_code"`
	InternalErrorMsg  string `json:"internal_error_msg"`
	Data              T      `json:"data"`
}

type ModelRequest struct {
	ModelID     string `json:"model_id,omitempty"`
	Version     string `json:"version,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
}

type VoiceSource struct {
	VoiceID   string `json:"voice_id,omitempty"`
	VoiceURL  string `json:"voice_url,omitempty"`
	VoiceData string `json:"voice_data,omitempty"`
	VoiceText string `json:"voice_text,omitempty"`
}

type AudioResult struct {
	Kind         string `json:"kind,omitempty"`
	AudioURL     string `json:"audio_url,omitempty"`
	AudioAssetID string `json:"audio_asset_id,omitempty"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
}

type TranscriptionRequest struct {
	ModelRequest
	URL               string `json:"url,omitempty"`
	AudioURL          string `json:"audio_url,omitempty"`
	InputAudioAssetID string `json:"input_audio_asset_id,omitempty"`
	AudioData         string `json:"audio_data,omitempty"`
	Language          string `json:"language,omitempty"`
	SamplingParams    any    `json:"sampling_params,omitempty"`
	MetaInfo          bool   `json:"meta_info,omitempty"`
	TimestampFormat   string `json:"timestamp_format,omitempty"`
	Filename          string `json:"filename,omitempty"`
	IsStream          bool   `json:"is_stream,omitempty"`
}

type TranscriptionSegment struct {
	Text      string  `json:"text,omitempty"`
	StartTime float64 `json:"start_time,omitempty"`
	EndTime   float64 `json:"end_time,omitempty"`
}

type ImageGenerationRequest struct {
	ModelRequest
	Model                string         `json:"model,omitempty"`
	Prompt               string         `json:"prompt"`
	ReferenceImages      []string       `json:"reference_images,omitempty"`
	RefImageURL          string         `json:"ref_image_url,omitempty"`
	RefImageURLs         []string       `json:"ref_image_urls,omitempty"`
	ImageURL             string         `json:"image_url,omitempty"`
	InputAssetID         string         `json:"input_asset_id,omitempty"`
	NegativePrompt       string         `json:"negative_prompt,omitempty"`
	Size                 string         `json:"size,omitempty"`
	Ratio                string         `json:"ratio,omitempty"`
	AspectRatio          string         `json:"aspect_ratio,omitempty"`
	Seed                 *int           `json:"seed,omitempty"`
	GuidanceScale        float64        `json:"guidance_scale,omitempty"`
	NumInferenceSteps    int            `json:"num_inference_steps,omitempty"`
	GeneratePreviewImage bool           `json:"generate_preview_image,omitempty"`
	OutputWatermark      map[string]any `json:"output_watermark,omitempty"`
	Extra                map[string]any `json:"extra,omitempty"`
}

type ImageGenerationResult struct {
	Kind          string `json:"kind,omitempty"`
	ImageURL      string `json:"image_url,omitempty"`
	ImageAssetID  string `json:"image_asset_id,omitempty"`
	ResultAssetID string `json:"result_asset_id,omitempty"`
}

func (mc *MaasClient) ImageGeneration(ctx context.Context, req ImageGenerationRequest) (*ModelResponse[ImageGenerationResult], error) {
	return submitModelTask[ImageGenerationRequest, ImageGenerationResult](ctx, mc.httpClient, imageGenerationPath, req)
}

type VideoGenerationRequest struct {
	ModelRequest
	Model           string         `json:"model,omitempty"`
	Prompt          string         `json:"prompt"`
	ReferenceImage  string         `json:"reference_image,omitempty"`
	ImageURL        string         `json:"image_url,omitempty"`
	ReferenceImages []string       `json:"reference_images,omitempty"`
	RefImageURL     string         `json:"ref_image_url,omitempty"`
	RefImageURLs    []string       `json:"ref_image_urls,omitempty"`
	InputAssetID    string         `json:"input_asset_id,omitempty"`
	ReferenceVideos []string       `json:"reference_videos,omitempty"`
	ReferenceAudios []string       `json:"reference_audios,omitempty"`
	LastFrameURL    string         `json:"last_frame_url,omitempty"`
	Duration        int            `json:"duration,omitempty"`
	DurationSec     int            `json:"duration_sec,omitempty"`
	AspectRatio     string         `json:"aspect_ratio,omitempty"`
	Ratio           string         `json:"ratio,omitempty"`
	Resolution      string         `json:"resolution,omitempty"`
	AudioSync       *bool          `json:"audio_sync,omitempty"`
	Seed            *int           `json:"seed,omitempty"`
	GenerateAudio   *bool          `json:"generate_audio,omitempty"`
	Watermark       *bool          `json:"watermark,omitempty"`
	ReturnLastFrame *bool          `json:"return_last_frame,omitempty"`
	ServiceTier     string         `json:"service_tier,omitempty"`
	OutputWatermark map[string]any `json:"output_watermark,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

type VideoGenerationResult struct {
	Kind                string `json:"kind,omitempty"`
	VideoURL            string `json:"video_url,omitempty"`
	VideoAssetID        string `json:"video_asset_id,omitempty"`
	WatermarkURL        string `json:"watermark_url,omitempty"`
	ResultAssetID       string `json:"result_asset_id,omitempty"`
	PreviewImageAssetID string `json:"preview_image_asset_id,omitempty"`
	PreviewImageURL     string `json:"preview_image_url,omitempty"`
	DurationMS          int64  `json:"duration_ms,omitempty"`
}

func (mc *MaasClient) VideoGeneration(ctx context.Context, req VideoGenerationRequest) (*ModelResponse[VideoGenerationResult], error) {
	return submitModelTask[VideoGenerationRequest, VideoGenerationResult](ctx, mc.httpClient, videoGenerationPath, req)
}

type TTSRequest struct {
	ModelRequest
	Text                string  `json:"text"`
	ExpectedDurationSec float64 `json:"expected_duration_sec,omitempty"`
	SamplingParams      any     `json:"sampling_params,omitempty"`
	MetaInfo            bool    `json:"meta_info,omitempty"`
	ExpiresIn           int     `json:"expires_in,omitempty"`
	ResponseFormat      string  `json:"response_format,omitempty"`
	VoiceSource
}

type TTSResult = AudioResult

func (mc *MaasClient) TTS(ctx context.Context, req TTSRequest) (*ModelResponse[TTSResult], error) {
	return submitModelTask[TTSRequest, TTSResult](ctx, mc.httpClient, ttsPath, req)
}

type TTSDRequest struct {
	ModelRequest
	Text                string        `json:"text"`
	Speakers            []VoiceSource `json:"speakers"`
	ExpectedDurationSec float64       `json:"expected_duration_sec,omitempty"`
	SamplingParams      any           `json:"sampling_params,omitempty"`
	MetaInfo            bool          `json:"meta_info,omitempty"`
	ExpiresIn           int           `json:"expires_in,omitempty"`
	ResponseFormat      string        `json:"response_format,omitempty"`
}

type TTSDResult = AudioResult

func (mc *MaasClient) TTSD(ctx context.Context, req TTSDRequest) (*ModelResponse[TTSDResult], error) {
	return submitModelTask[TTSDRequest, TTSDResult](ctx, mc.httpClient, ttsdPath, req)
}

type VoiceGeneratorRequest struct {
	ModelRequest
	Text           string `json:"text"`
	Instruction    string `json:"instruction"`
	SamplingParams any    `json:"sampling_params,omitempty"`
	MetaInfo       bool   `json:"meta_info,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type VoiceGeneratorResult = AudioResult

func (mc *MaasClient) VoiceGenerator(ctx context.Context, req VoiceGeneratorRequest) (*ModelResponse[VoiceGeneratorResult], error) {
	return submitModelTask[VoiceGeneratorRequest, VoiceGeneratorResult](ctx, mc.httpClient, voiceGeneratorPath, req)
}

type VoiceConvertRequest struct {
	ModelRequest
	InputAudio        string  `json:"input_audio,omitempty"`
	InputAudioAssetID string  `json:"input_audio_asset_id,omitempty"`
	InputAudioData    string  `json:"input_audio_data,omitempty"`
	RefAudio          string  `json:"ref_audio,omitempty"`
	RefAudioAssetID   string  `json:"ref_audio_asset_id,omitempty"`
	RefAudioData      string  `json:"ref_audio_data,omitempty"`
	Ratio             float64 `json:"ratio,omitempty"`
	MetaInfo          bool    `json:"meta_info,omitempty"`
	ExpiresIn         int     `json:"expires_in,omitempty"`
	ResponseFormat    string  `json:"response_format,omitempty"`
}

type VoiceConvertResult = AudioResult

func (mc *MaasClient) VoiceConvert(ctx context.Context, req VoiceConvertRequest) (*ModelResponse[VoiceConvertResult], error) {
	return submitModelTask[VoiceConvertRequest, VoiceConvertResult](ctx, mc.httpClient, voiceConvertPath, req)
}

type SpeechEnhanceRequest struct {
	ModelRequest
	Audio             string `json:"audio,omitempty"`
	InputAudioAssetID string `json:"input_audio_asset_id,omitempty"`
	AudioData         string `json:"audio_data,omitempty"`
	MetaInfo          bool   `json:"meta_info,omitempty"`
	ExpiresIn         int    `json:"expires_in,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
}

type SpeechEnhanceResult = AudioResult

func (mc *MaasClient) SpeechEnhance(ctx context.Context, req SpeechEnhanceRequest) (*ModelResponse[SpeechEnhanceResult], error) {
	return submitModelTask[SpeechEnhanceRequest, SpeechEnhanceResult](ctx, mc.httpClient, speechEnhancePath, req)
}

type SoundEffectGenerationRequest struct {
	ModelRequest
	Prompt         string `json:"prompt"`
	Seconds        *int   `json:"seconds,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type SoundEffectGenerationResult = AudioResult

func (mc *MaasClient) SoundEffectGeneration(ctx context.Context, req SoundEffectGenerationRequest) (*ModelResponse[SoundEffectGenerationResult], error) {
	return submitModelTask[SoundEffectGenerationRequest, SoundEffectGenerationResult](ctx, mc.httpClient, soundEffectGenerationPath, req)
}

type AudioTranscriptionRequest struct {
	TranscriptionRequest
}

type AudioTranscriptionResult struct {
	Kind     string                 `json:"kind,omitempty"`
	Text     string                 `json:"text,omitempty"`
	Segments []TranscriptionSegment `json:"segments,omitempty"`
}

func (mc *MaasClient) AudioTranscription(ctx context.Context, req AudioTranscriptionRequest) (*ModelResponse[AudioTranscriptionResult], error) {
	return submitModelTask[AudioTranscriptionRequest, AudioTranscriptionResult](ctx, mc.httpClient, audioTranscriptionPath, req)
}

type TranscriptionDiarizationRequest struct {
	TranscriptionRequest
}

type TranscriptionDiarizationResult = AudioTranscriptionResult

func (mc *MaasClient) TranscriptionDiarization(ctx context.Context, req TranscriptionDiarizationRequest) (*ModelResponse[TranscriptionDiarizationResult], error) {
	return submitModelTask[TranscriptionDiarizationRequest, TranscriptionDiarizationResult](ctx, mc.httpClient, transcriptionDiarizationPath, req)
}

type ModelTask struct {
	TaskID       string          `json:"task_id"`
	Object       string          `json:"object"`
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	ModelID      string          `json:"model_id,omitempty"`
	ModelVersion string          `json:"model_version,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	Error        json.RawMessage `json:"error,omitempty"`
	CreatedAt    string          `json:"created_at,omitempty"`
	UpdatedAt    string          `json:"updated_at,omitempty"`
}

func submitModelTask[Request, Result any](
	ctx context.Context,
	client *httpclient.Client,
	path string,
	req Request,
) (*ModelResponse[Result], error) {
	resp, err := client.SendJSON(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}

	var envelope maasEnvelope[ModelResponse[Result]]
	if err := resp.DecodeJSON(&envelope); err != nil {
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, resp.ToError("maas")
		}
		return nil, fmt.Errorf("decode maas response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || envelope.ErrorCode != 0 {
		return nil, fmt.Errorf(
			"maas request failed: status=%d error_code=%d error_msg=%s internal_error_code=%d internal_error_msg=%s",
			resp.StatusCode,
			envelope.ErrorCode,
			envelope.ErrorMsg,
			envelope.InternalErrorCode,
			envelope.InternalErrorMsg,
		)
	}
	return &envelope.Data, nil
}

func (mc *MaasClient) GetTask(ctx context.Context, taskID string) (*ModelTask, error) {
	resp, err := mc.httpClient.GetJSON(ctx, modelTaskPath+url.PathEscape(taskID))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, resp.ToError("get maas task")
	}

	var task ModelTask
	if err := resp.DecodeJSON(&task); err != nil {
		return nil, fmt.Errorf("decode maas task: %w", err)
	}
	return &task, nil
}
