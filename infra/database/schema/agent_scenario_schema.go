package schema

import "time"

type AgentScenarioSchema struct {
	ID          string    `gorm:"primaryKey;type:uuid"`
	WorkspaceID string    `gorm:"type:uuid;not null;index:idx_agent_scenarios_agent,priority:1"`
	AgentID     string    `gorm:"type:uuid;not null;index:idx_agent_scenarios_agent,priority:2"`
	Name        string    `gorm:"size:120;not null"`
	Messages    string    `gorm:"type:jsonb;not null;default:'[]'"`
	Mocks       string    `gorm:"type:jsonb;not null;default:'{}'"`
	Assertions  string    `gorm:"type:jsonb;not null;default:'[]'"`
	SourceRunID string    `gorm:"size:64;not null;default:''"`
	CreatedBy   string    `gorm:"size:64;not null;default:''"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (AgentScenarioSchema) TableName() string { return "agent_scenarios" }

type AgentScenarioRunSchema struct {
	ID          string    `gorm:"primaryKey;type:uuid"`
	ScenarioID  string    `gorm:"type:uuid;not null;index:idx_agent_scenario_runs_scenario,priority:1"`
	WorkspaceID string    `gorm:"type:uuid;not null"`
	AgentID     string    `gorm:"type:uuid;not null"`
	Passed      bool      `gorm:"not null;default:false"`
	Results     string    `gorm:"type:jsonb;not null;default:'[]'"`
	Turns       string    `gorm:"type:jsonb;not null;default:'[]'"`
	RunIDs      string    `gorm:"column:run_ids;type:jsonb;not null;default:'[]'"`
	Error       string    `gorm:"type:text;not null;default:''"`
	StartedAt   time.Time `gorm:"type:timestamptz;not null;index:idx_agent_scenario_runs_scenario,priority:2,sort:desc;index:idx_agent_scenario_runs_started"`
	FinishedAt  time.Time `gorm:"type:timestamptz;not null"`
}

func (AgentScenarioRunSchema) TableName() string { return "agent_scenario_runs" }
