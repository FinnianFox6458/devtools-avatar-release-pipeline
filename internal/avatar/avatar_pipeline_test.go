package avatar

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeOperations struct {
	failAt  string
	calls   []string
	updated bool
}

func (f *fakeOperations) Upload(context.Context, []byte, string, string) (string, error) {
	return f.step("upload", "uploaded-image")
}

func (f *fakeOperations) SmartCrop(context.Context, string, string, string) (string, error) {
	return f.step("crop", "cropped-image")
}

func (f *fakeOperations) Resize(context.Context, string, string) (string, error) {
	return f.step("optimize", "stored-avatar")
}

func (f *fakeOperations) UpdateUserAvatar(context.Context, string, string, string) error {
	f.calls = append(f.calls, "release")
	if f.failAt == "release" {
		return errors.New("test rejection")
	}
	f.updated = true
	return nil
}

func (f *fakeOperations) step(name, result string) (string, error) {
	f.calls = append(f.calls, name)
	if f.failAt == name {
		return "", errors.New("test rejection")
	}
	return result, nil
}

func TestPipelinePublishesOnlyAfterOptimization(t *testing.T) {
	tests := []struct {
		name      string
		failAt    string
		wantCalls []string
		published bool
		state     string
	}{
		{name: "complete pipeline", wantCalls: []string{"upload", "crop", "optimize", "release"}, published: true, state: "published"},
		{name: "optimization rejected", failAt: "optimize", wantCalls: []string{"upload", "crop", "optimize"}, published: false, state: "blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operations := &fakeOperations{failAt: tt.failAt}
			pipeline := NewPipeline(operations)
			pipeline.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
			result, _ := pipeline.Run(context.Background(), AvatarRequest{
				UserID: "user-7", BuildID: "build-42", ReleaseID: "release-9",
				OperationID: "operation-3", Filename: "avatar.png", File: []byte("image"),
			})

			if !reflect.DeepEqual(operations.calls, tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", operations.calls, tt.wantCalls)
			}
			if operations.updated != tt.published {
				t.Fatalf("updated = %v, want %v", operations.updated, tt.published)
			}
			if result.Release.State != tt.state {
				t.Fatalf("release state = %q, want %q", result.Release.State, tt.state)
			}
		})
	}
}
