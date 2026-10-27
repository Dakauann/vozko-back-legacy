package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/testinput"
	"vozko/infra/database/schema"
	agenttool_repository "vozko/infra/repositories/agenttool"
	automationneeds_repository "vozko/infra/repositories/automationneeds"
)

func (r *repository) FindPublished(agentID string) (*agent.Agent, error) {
	live, err := r.FindByID(agentID)
	if err != nil {
		return nil, err
	}
	var row schema.Agent
	if err := r.db.Select("id", "published_version").Where("id = ?", agentID).First(&row).Error; err != nil {
		return nil, err
	}
	if row.PublishedVersion == nil || *row.PublishedVersion == 0 {
		return nil, agent.ErrAgentNotPublished
	}
	version, err := r.VersionOf(context.Background(), agentID, *row.PublishedVersion)
	if err != nil {
		return nil, err
	}
	tools, err := agenttool_repository.LoadPinned(r.db, agentID, version.Snapshot.Tools)
	if err != nil {
		return nil, err
	}
	published := version.Snapshot.Apply(live, tools)
	published.RunningVersion = version.Version
	return published, nil
}

func (r *repository) PublishVersion(ctx context.Context, agentID string, snapshot agent.Snapshot, publishedBy string) (agent.Version, error) {
	version, _, err := r.PublishVersionSwitchingOff(ctx, agentID, snapshot, publishedBy, "", nil)
	return version, err
}

func (r *repository) PublishVersionSwitchingOff(ctx context.Context, agentID string, snapshot agent.Snapshot, publishedBy, workspaceID string, offs []automationneeds.SwitchOff) (agent.Version, []string, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return agent.Version{}, nil, err
	}
	var out agent.Version
	var containers []string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Agent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "workspace_id").Where("id = ?", agentID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return agent.ErrAgentNotFound
			}
			return err
		}
		var latest int
		if err := tx.Model(&schema.AgentVersion{}).Where("agent_id = ?", agentID).Select("COALESCE(MAX(version), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		version := schema.AgentVersion{ID: uuid.NewString(), AgentID: agentID, Version: latest + 1, WorkspaceID: row.WorkspaceID, Snapshot: string(raw), PublishedBy: publishedBy, PublishedAt: now}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if err := tx.Model(&schema.Agent{}).Where("id = ?", agentID).UpdateColumn("published_version", version.Version).Error; err != nil {
			return err
		}
		out = agent.Version{AgentID: agentID, Version: version.Version, Snapshot: snapshot, PublishedBy: publishedBy, PublishedAt: now}
		if len(offs) == 0 {
			return nil
		}
		if row.WorkspaceID != workspaceID {
			return agent.ErrAgentNotFound
		}
		switched, err := automationneeds_repository.SwitchOffIn(tx, workspaceID, offs, publishedBy, now)
		containers = switched
		return err
	})
	return out, containers, err
}

func (r *repository) CurrentVersion(ctx context.Context, agentID string) (*agent.Version, error) {
	var row schema.Agent
	if err := r.db.WithContext(ctx).Select("id", "published_version").Where("id = ?", agentID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, agent.ErrAgentNotFound
		}
		return nil, err
	}
	if row.PublishedVersion == nil || *row.PublishedVersion == 0 {
		return nil, nil
	}
	version, err := r.VersionOf(ctx, agentID, *row.PublishedVersion)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *repository) Versions(ctx context.Context, agentID string) ([]agent.Version, error) {
	var rows []schema.AgentVersion
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Order("version DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]agent.Version, 0, len(rows))
	for _, row := range rows {
		version, err := versionFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, nil
}

func (r *repository) VersionOf(ctx context.Context, agentID string, version int) (agent.Version, error) {
	var row schema.AgentVersion
	err := r.db.WithContext(ctx).Where("agent_id = ? AND version = ?", agentID, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agent.Version{}, agent.ErrAgentVersionMissing
	}
	if err != nil {
		return agent.Version{}, err
	}
	return versionFrom(row)
}

func versionFrom(row schema.AgentVersion) (agent.Version, error) {
	var snapshot agent.Snapshot
	if err := json.Unmarshal([]byte(row.Snapshot), &snapshot); err != nil {
		return agent.Version{}, err
	}
	return agent.Version{AgentID: row.AgentID, Version: row.Version, Snapshot: snapshot, PublishedBy: row.PublishedBy, PublishedAt: row.PublishedAt}, nil
}

var (
	_ agent.PublishedReader   = (*repository)(nil)
	_ agent.VersionRepository = (*repository)(nil)
)

func (r *repository) UnpublishedAgentIDs(ctx context.Context, afterID string, limit int) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&schema.Agent{}).
		Where("id > ?::uuid AND published_version IS NULL", afterID).
		Order("id").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

var _ agent.UnpublishedScanner = (*repository)(nil)

func neverPublished() *int {
	zero := 0
	return &zero
}

func (r *repository) PinnedTools(ctx context.Context, agentID string, pins []agent.ToolPin) ([]agent.ToolBinding, error) {
	return agenttool_repository.LoadPinned(r.db.WithContext(ctx), agentID, pins)
}

func (r *repository) SaveToolTestInputs(ctx context.Context, agentID, toolID string, inputs []testinput.TestInput) error {
	return agenttool_repository.SaveTestInputs(r.db.WithContext(ctx), agentID, toolID, inputs)
}
