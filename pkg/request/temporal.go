package request

import (
	"context"

	"go.temporal.io/sdk/converter"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

const temporalHeader = "matrix-request-info"

// TemporalContextPropagator 通过 Temporal 传播请求信息。
type TemporalContextPropagator struct{}

func (TemporalContextPropagator) Inject(ctx context.Context, writer temporalworkflow.HeaderWriter) error {
	info, ok := FromContext(ctx)
	if !ok {
		return nil
	}
	return writeTemporalHeader(writer, info)
}

func (TemporalContextPropagator) Extract(ctx context.Context, reader temporalworkflow.HeaderReader) (context.Context, error) {
	info, ok, err := readTemporalHeader(reader)
	if err != nil || !ok {
		return ctx, err
	}
	return WithInfo(ctx, info), nil
}

func (TemporalContextPropagator) InjectFromWorkflow(ctx temporalworkflow.Context, writer temporalworkflow.HeaderWriter) error {
	info, ok := ctx.Value(contextKey{}).(Info)
	if !ok {
		return nil
	}
	return writeTemporalHeader(writer, info)
}

func (TemporalContextPropagator) ExtractToWorkflow(ctx temporalworkflow.Context, reader temporalworkflow.HeaderReader) (temporalworkflow.Context, error) {
	info, ok, err := readTemporalHeader(reader)
	if err != nil || !ok {
		return ctx, err
	}
	return temporalworkflow.WithValue(ctx, contextKey{}, info), nil
}

func writeTemporalHeader(writer temporalworkflow.HeaderWriter, info Info) error {
	payload, err := converter.GetDefaultDataConverter().ToPayload(info)
	if err != nil {
		return err
	}
	writer.Set(temporalHeader, payload)
	return nil
}

func readTemporalHeader(reader temporalworkflow.HeaderReader) (Info, bool, error) {
	payload, ok := reader.Get(temporalHeader)
	if !ok {
		return Info{}, false, nil
	}

	var info Info
	if err := converter.GetDefaultDataConverter().FromPayload(payload, &info); err != nil {
		return Info{}, false, err
	}
	return info, true, nil
}
