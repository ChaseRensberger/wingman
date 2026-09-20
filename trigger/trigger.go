// Package trigger defines scheduled inputs to the Wingman runtime.
package trigger

import (
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

// ErrNotFound identifies an absent trigger.
var ErrNotFound = errors.New("trigger not found")

// ErrConflict identifies a trigger changed by another operation.
var ErrConflict = errors.New("trigger changed; reload it and try again")

// Source describes when a trigger fires.
type Source struct {
	Type       string `json:"type" enum:"cron"`
	Expression string `json:"expression"`
	TimeZone   string `json:"time_zone"`
}

// Target describes the fresh Session created for each execution.
type Target struct {
	AgentID          string `json:"agent_id"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	ModelRef         string `json:"model_ref,omitempty"`
}

// Config is the user-editable definition of a trigger.
type Config struct {
	Name    string `json:"name"`
	Source  Source `json:"source"`
	Target  Target `json:"target"`
	Prompt  string `json:"prompt"`
	Enabled bool   `json:"enabled"`
}

// Trigger is a persistent, client-owned source of Session runs.
type Trigger struct {
	Config
	ID             string      `json:"id"`
	ClientID       string      `json:"client_id"`
	Version        int64       `json:"version"`
	NextFireAt     *time.Time  `json:"next_fire_at,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	RunCount       int64       `json:"run_count"`
	LastOccurrence *Occurrence `json:"last_occurrence,omitempty"`
}

// Occurrence records why a trigger fired and the outcome of its submission.
type Occurrence struct {
	ID          string     `json:"id"`
	TriggerID   string     `json:"trigger_id"`
	TriggerName string     `json:"trigger_name"`
	Source      string     `json:"source" enum:"cron,manual"`
	RequestID   string     `json:"request_id"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	CreatedAt   time.Time  `json:"created_at"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
	SessionID   string     `json:"session_id,omitempty"`
	RunID       string     `json:"run_id,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Schedule validates a cron source with minute precision and an explicit IANA zone.
func (s Source) Schedule() (cron.Schedule, error) {
	if s.Type != "cron" {
		return nil, errors.New("source type must be cron")
	}
	if s.TimeZone == "" || s.TimeZone == "Local" {
		return nil, errors.New("an explicit time zone is required, such as UTC or America/New_York")
	}
	location, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("invalid time zone: %s", s.TimeZone)
	}
	if len(strings.Fields(s.Expression)) != 5 {
		return nil, errors.New("cron requires five fields: minute, hour, day, month, weekday")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(s.Expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	schedule.(*cron.SpecSchedule).Location = location
	return schedule, nil
}

// Next returns the first scheduled time strictly after now.
func (s Source) Next(now time.Time) (time.Time, error) {
	schedule, err := s.Schedule()
	if err != nil {
		return time.Time{}, err
	}
	next := schedule.Next(now)
	if next.IsZero() {
		return time.Time{}, errors.New("cron expression has no occurrence in the next five years")
	}
	return next.UTC(), nil
}

// Validate checks a trigger definition without accessing runtime resources.
func (c Config) Validate(now time.Time) error {
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 120 {
		return errors.New("name is required and must be at most 120 bytes")
	}
	if strings.TrimSpace(c.Prompt) == "" || len(c.Prompt) > 65536 {
		return errors.New("prompt is required and must be at most 65536 bytes")
	}
	if c.Target.AgentID == "" {
		return errors.New("agent_id is required")
	}
	if c.Target.WorkspaceID != "" && c.Target.WorkingDirectory != "" {
		return errors.New("workspace_id and working_directory cannot both be set")
	}
	_, err := c.Source.Next(now)
	return err
}
