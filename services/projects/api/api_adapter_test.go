package api

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/services/projects/api/spec"
)

func TestApiToClustersBackupConfig(t *testing.T) {
	tests := []struct {
		name           string
		backupConfig   *spec.BackupConfiguration
		backupsEnabled bool
		usePgBackRest  bool
		wantSchedule   bool
		wantMethod     string
	}{
		{
			name:           "backups disabled returns config with BackupsEnabled false",
			backupConfig:   &spec.BackupConfiguration{BackupTime: new("0:14:30")},
			backupsEnabled: false,
			wantSchedule:   false,
		},
		{
			name:           "backups enabled with no config returns default barman",
			backupConfig:   nil,
			backupsEnabled: true,
			wantSchedule:   true,
			wantMethod:     BackupMethodBarman,
		},
		{
			name:           "backups enabled with config returns schedule barman",
			backupConfig:   &spec.BackupConfiguration{BackupTime: new("0:14:30")},
			backupsEnabled: true,
			wantSchedule:   true,
			wantMethod:     BackupMethodBarman,
		},
		{
			name:           "pgbackrest with no config returns default pgbackrest",
			backupConfig:   nil,
			backupsEnabled: true,
			usePgBackRest:  true,
			wantSchedule:   true,
			wantMethod:     BackupMethodPgBackRest,
		},
		{
			name:           "pgbackrest with config returns schedule pgbackrest",
			backupConfig:   &spec.BackupConfiguration{BackupTime: new("0:14:30")},
			backupsEnabled: true,
			usePgBackRest:  true,
			wantSchedule:   true,
			wantMethod:     BackupMethodPgBackRest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := apiToClustersBackupConfig(tt.backupConfig, tt.backupsEnabled, tt.usePgBackRest)

			assert.Equal(t, tt.backupsEnabled, result.BackupsEnabled)

			if tt.wantSchedule {
				assert.NotEmpty(t, result.BackupSchedule)
				assert.NotEmpty(t, result.BackupRetention)
			} else {
				assert.Empty(t, result.BackupSchedule)
				assert.Empty(t, result.BackupRetention)
			}

			if tt.wantMethod != "" {
				assert.Equal(t, tt.wantMethod, result.BackupMethod)
			}
		})
	}
}

func Test_generateSchedule(t *testing.T) {
	tests := []struct {
		name     string
		cron     string
		expected string
	}{
		{
			name:     "daily backup at 2:30 PM",
			cron:     "0 30 14 * * *",
			expected: "*:14:30",
		},
		{
			name:     "Sunday backup at 11:45 PM",
			cron:     "0 45 23 * * 0",
			expected: "0:23:45",
		},
		{
			name:     "Monday backup at 6:15 AM",
			cron:     "0 15 6 * * 1",
			expected: "1:06:15",
		},
		{
			name:     "Tuesday backup at 3:00 AM",
			cron:     "0 0 3 * * 2",
			expected: "2:03:00",
		},
		{
			name:     "Wednesday backup at midnight",
			cron:     "0 0 0 * * 3",
			expected: "3:00:00",
		},
		{
			name:     "Thursday backup with single digit minute",
			cron:     "0 5 12 * * 4",
			expected: "4:12:05",
		},
		{
			name:     "Friday backup with single digit hour",
			cron:     "0 30 9 * * 5",
			expected: "5:09:30",
		},
		{
			name:     "Saturday backup at 23:59",
			cron:     "0 59 23 * * 6",
			expected: "6:23:59",
		},
		{
			name:     "daily backup at midnight",
			cron:     "0 0 0 * * *",
			expected: "*:00:00",
		},
		{
			name:     "daily backup at noon",
			cron:     "0 0 12 * * *",
			expected: "*:12:00",
		},
		{
			name:     "invalid cron format - too few fields",
			cron:     "0 30 14",
			expected: "*:00:00",
		},
		{
			name:     "invalid cron format - too many fields",
			cron:     "0 30 14 * * * *",
			expected: "*:00:00",
		},
		{
			name:     "invalid cron format - empty string",
			cron:     "",
			expected: "*:00:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSchedule(tt.cron)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_generateCron(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		expected string
	}{
		{
			name:     "daily backup at 2:30 PM",
			schedule: "*:14:30",
			expected: "0 30 14 * * *",
		},
		{
			name:     "Sunday backup at 11:45 PM",
			schedule: "0:23:45",
			expected: "0 45 23 * * 0",
		},
		{
			name:     "Monday backup at 6:15 AM",
			schedule: "1:06:15",
			expected: "0 15 06 * * 1",
		},
		{
			name:     "Tuesday backup at 3:00 AM",
			schedule: "2:03:00",
			expected: "0 00 03 * * 2",
		},
		{
			name:     "Wednesday backup at midnight",
			schedule: "3:00:00",
			expected: "0 00 00 * * 3",
		},
		{
			name:     "Thursday backup with single digit minute",
			schedule: "4:12:05",
			expected: "0 05 12 * * 4",
		},
		{
			name:     "Friday backup with single digit hour",
			schedule: "5:09:30",
			expected: "0 30 09 * * 5",
		},
		{
			name:     "Saturday backup at 23:59",
			schedule: "6:23:59",
			expected: "0 59 23 * * 6",
		},
		{
			name:     "daily backup at midnight",
			schedule: "*:00:00",
			expected: "0 00 00 * * *",
		},
		{
			name:     "daily backup at noon",
			schedule: "*:12:00",
			expected: "0 00 12 * * *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateCron(tt.schedule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test that generateCron and generateSchedule are inverses of each other
func Test_generateCron_generateSchedule_inverse(t *testing.T) {
	schedules := []string{
		"*:14:30",
		"0:23:45",
		"1:06:15",
		"2:03:00",
		"3:00:00",
		"4:12:05",
		"5:09:30",
		"6:23:59",
		"*:00:00",
		"*:12:00",
	}

	for _, schedule := range schedules {
		t.Run("schedule_"+schedule, func(t *testing.T) {
			cron := generateCron(schedule)
			backToSchedule := generateSchedule(cron)
			assert.Equal(t, schedule, backToSchedule, "generateCron and generateSchedule should be inverses")
		})
	}
}

func Test_parseRecoveryWindow(t *testing.T) {
	tests := []struct {
		name         string
		window       *clustersv1.GetRecoveryWindowResponse
		wantEarliest *time.Time
		wantLatest   *time.Time
		wantErr      bool
		errContains  string
	}{
		{
			name: "successful parsing with valid timestamps",
			window: &clustersv1.GetRecoveryWindowResponse{
				FirstRecoverabilityPoint: "2023-01-01T10:00:00Z",
				LastSuccessfulBackup:     "2023-01-02T15:30:00Z",
			},
			wantEarliest: new(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			wantLatest:   new(time.Date(2023, 1, 2, 15, 30, 0, 0, time.UTC)),
			wantErr:      false,
		},
		{
			name: "last recoverability point wins over last successful backup",
			window: &clustersv1.GetRecoveryWindowResponse{
				FirstRecoverabilityPoint: "2023-01-01T10:00:00Z",
				LastRecoverabilityPoint:  "2023-01-02T16:00:00Z",
				LastSuccessfulBackup:     "2023-01-02T15:30:00Z",
			},
			wantEarliest: new(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			wantLatest:   new(time.Date(2023, 1, 2, 16, 0, 0, 0, time.UTC)),
			wantErr:      false,
		},
		{
			name:         "nil window",
			window:       nil,
			wantEarliest: nil,
			wantLatest:   nil,
			wantErr:      false,
		},
		{
			name: "invalid earliest restore timestamp",
			window: &clustersv1.GetRecoveryWindowResponse{
				FirstRecoverabilityPoint: "invalid-timestamp",
				LastSuccessfulBackup:     "2023-01-02T15:30:00Z",
			},
			wantEarliest: nil,
			wantLatest:   nil,
			wantErr:      true,
			errContains:  "unexpected timestamp for earliest restore time",
		},
		{
			name: "invalid latest restore timestamp",
			window: &clustersv1.GetRecoveryWindowResponse{
				FirstRecoverabilityPoint: "2023-01-01T10:00:00Z",
				LastSuccessfulBackup:     "invalid-timestamp",
			},
			wantEarliest: nil,
			wantLatest:   nil,
			wantErr:      true,
			errContains:  "unexpected timestamp for latest restore time",
		},
		{
			name:         "empty timestamp strings",
			window:       &clustersv1.GetRecoveryWindowResponse{},
			wantEarliest: nil,
			wantLatest:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEarliest, gotLatest, err := parseRecoveryWindow(tt.window)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseRecoveryWindow() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("parseRecoveryWindow() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("parseRecoveryWindow() unexpected error = %v", err)
				return
			}

			if (gotEarliest == nil) != (tt.wantEarliest == nil) {
				t.Errorf("parseRecoveryWindow() gotEarliest = %v, want %v", gotEarliest, tt.wantEarliest)
				return
			}
			if gotEarliest != nil && tt.wantEarliest != nil && !gotEarliest.Equal(*tt.wantEarliest) {
				t.Errorf("parseRecoveryWindow() gotEarliest = %v, want %v", gotEarliest, tt.wantEarliest)
			}

			if (gotLatest == nil) != (tt.wantLatest == nil) {
				t.Errorf("parseRecoveryWindow() gotLatest = %v, want %v", gotLatest, tt.wantLatest)
				return
			}
			if gotLatest != nil && tt.wantLatest != nil && !gotLatest.Equal(*tt.wantLatest) {
				t.Errorf("parseRecoveryWindow() gotLatest = %v, want %v", gotLatest, tt.wantLatest)
			}
		})
	}
}
