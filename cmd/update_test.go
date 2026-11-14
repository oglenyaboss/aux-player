package cmd

import (
	"strings"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/updater"
)

func TestUpdateReport(t *testing.T) {
	tests := []struct {
		name    string
		res     updater.Result
		want    string
		wantErr bool
	}{
		{"failed", updater.Result{Outcome: updater.OutcomeFailed}, "", true},
		{"current", updater.Result{Outcome: updater.OutcomeCurrent, Tag: "v1.2.0"}, "newest release", false},
		{"installed", updater.Result{Outcome: updater.OutcomeInstalled, Tag: "v1.3.0"}, "updated to v1.3.0", false},
		{"dev build", updater.Result{Outcome: updater.OutcomeDisabled, Tag: "v1.3.0"}, "build from source", false},
		{"manual", updater.Result{Outcome: updater.OutcomeManual, Tag: "v1.3.0"}, "reinstall", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := updateReport("1.2.0", tt.res)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want it to contain %q", got, tt.want)
			}
			if strings.Contains(got, "--no-update") {
				t.Errorf("got %q: aux update never runs with --no-update", got)
			}
		})
	}
}
