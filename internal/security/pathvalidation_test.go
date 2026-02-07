package security

import (
	"path/filepath"
	"testing"
)

func TestValidateConfigPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"Valid relative path", "config.yml", false},
		{"Valid nested path", "configs/snapshot-tester.yml", false},
		{"Directory traversal", "../../../etc/passwd", true},
		{"Directory traversal with clean", "config/../../passwd", true},
		{"Empty path", "", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSnapshotPath(t *testing.T) {
	snapshotDir := "/tmp/snapshots"
	
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"Valid path within dir", "/tmp/snapshots/test.json", false},
		{"Valid nested path", "/tmp/snapshots/service/POST_users/001.json", false},
		{"Path traversal outside", "/tmp/other/file.json", true},
		{"Path traversal with ..", "/tmp/snapshots/../../../etc/passwd", true},
		{"Empty path", "", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSnapshotPath(tt.path, snapshotDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSnapshotPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"Clean path", "config.yml", "config.yml"},
		{"Path with ..", "../config.yml", "config.yml"},
		{"Multiple ..", "../../config.yml", "config.yml"},
		{"Nested valid", "configs/test.yml", filepath.Join("configs", "test.yml")},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizePath(tt.path)
			if got != tt.want {
				t.Errorf("SanitizePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
