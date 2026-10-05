package avatar

import (
	"context"
	"fmt"
	"time"
)

type InfraiOperations interface {
	Upload(context.Context, []byte, string, string) (string, error)
	SmartCrop(context.Context, string, string, string) (string, error)
	Resize(context.Context, string, string) (string, error)
	UpdateUserAvatar(context.Context, string, string, string) error
}

type AvatarRequest struct {
	UserID      string
	BuildID     string
	ReleaseID   string
	OperationID string
	Filename    string
	File        []byte
}

type BuildEvent struct {
	BuildID string    `json:"build_id"`
	Stage   string    `json:"stage"`
	State   string    `json:"state"`
	At      time.Time `json:"at"`
}

type ReleaseOperation struct {
	ReleaseID string `json:"release_id"`
	Operation string `json:"operation"`
	State     string `json:"state"`
	Image     string `json:"image,omitempty"`
}

type Diagnostic struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type PipelineResult struct {
	UserID      string           `json:"user_id"`
	Avatar      string           `json:"avatar,omitempty"`
	Events      []BuildEvent     `json:"events"`
	Release     ReleaseOperation `json:"release"`
	Diagnostics []Diagnostic     `json:"diagnostics"`
}

type Pipeline struct {
	infrai InfraiOperations
	now    func() time.Time
}

func NewPipeline(infrai InfraiOperations) *Pipeline {
	return &Pipeline{infrai: infrai, now: time.Now}
}

func (p *Pipeline) Run(ctx context.Context, input AvatarRequest) (PipelineResult, error) {
	result := PipelineResult{
		UserID:      input.UserID,
		Release:     ReleaseOperation{ReleaseID: input.ReleaseID, Operation: "publish_avatar", State: "pending"},
		Diagnostics: []Diagnostic{},
	}

	uploaded, err := p.runStage(ctx, &result, input.BuildID, "upload", func() (string, error) {
		return p.infrai.Upload(ctx, input.File, input.Filename, input.OperationID+":upload")
	})
	if err != nil {
		return result, err
	}
	cropped, err := p.runStage(ctx, &result, input.BuildID, "crop", func() (string, error) {
		return p.infrai.SmartCrop(ctx, uploaded, "1:1", input.OperationID+":crop")
	})
	if err != nil {
		return result, err
	}
	optimized, err := p.runStage(ctx, &result, input.BuildID, "optimize", func() (string, error) {
		return p.infrai.Resize(ctx, cropped, input.OperationID+":optimize")
	})
	if err != nil {
		return result, err
	}

	p.event(&result, input.BuildID, "release", "started")
	if err := p.infrai.UpdateUserAvatar(ctx, input.UserID, optimized, input.OperationID+":release"); err != nil {
		p.failed(&result, input.BuildID, "release", err)
		return result, fmt.Errorf("release avatar: %w", err)
	}
	p.event(&result, input.BuildID, "release", "succeeded")
	result.Avatar = optimized
	result.Release.State = "published"
	result.Release.Image = optimized
	return result, nil
}

func (p *Pipeline) runStage(ctx context.Context, result *PipelineResult, buildID, stage string, work func() (string, error)) (string, error) {
	p.event(result, buildID, stage, "started")
	value, err := work()
	if err != nil {
		p.failed(result, buildID, stage, err)
		return "", fmt.Errorf("%s avatar: %w", stage, err)
	}
	p.event(result, buildID, stage, "succeeded")
	return value, nil
}

func (p *Pipeline) event(result *PipelineResult, buildID, stage, state string) {
	result.Events = append(result.Events, BuildEvent{BuildID: buildID, Stage: stage, State: state, At: p.now().UTC()})
}

func (p *Pipeline) failed(result *PipelineResult, buildID, stage string, err error) {
	p.event(result, buildID, stage, "failed")
	result.Release.State = "blocked"
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Stage: stage, Message: err.Error()})
}
