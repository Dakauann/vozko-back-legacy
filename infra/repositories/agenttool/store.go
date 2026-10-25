package agenttool_repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/testinput"
	"vozko/infra/database/schema"
)

type SyncRequest struct {
	AgentID     string
	WorkspaceID string
	Actor       string
	Bindings    []agent.ToolBinding
	Expected    map[string]int
	Now         time.Time
}

func LoadBindings(db *gorm.DB, agentID string) ([]agent.ToolBinding, error) {
	var rows []schema.AgentTool
	if err := db.Where("agent_id = ?", agentID).Order("position, created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]agent.ToolBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, bindingOf(row))
	}
	return out, nil
}

func SyncTx(tx *gorm.DB, req SyncRequest) error {
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	var live []schema.AgentTool
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id = ?", req.AgentID).Find(&live).Error; err != nil {
		return err
	}
	stored := make(map[string]schema.AgentTool, len(live))
	for _, row := range live {
		stored[row.ID] = row
	}
	for id, revision := range req.Expected {
		row, ok := stored[id]
		if !ok {
			return fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, id)
		}
		if row.Revision != revision {
			return fmt.Errorf("%w: %s at revision %d, sent %d", agenttool.ErrRevisionConflict, id, row.Revision, revision)
		}
	}
	keep := make(map[string]bool, len(req.Bindings))
	for _, b := range req.Bindings {
		if b.ID == "" {
			return errors.New("agenttool store: every tool needs an id before it is saved")
		}
		keep[b.ID] = true
	}
	for _, row := range live {
		if !keep[row.ID] {
			if err := tx.Delete(&schema.AgentTool{}, "id = ?", row.ID).Error; err != nil {
				return err
			}
		}
	}
	for _, b := range req.Bindings {
		if row, ok := stored[b.ID]; ok && row.Name != b.Function {
			if err := tx.Model(&schema.AgentTool{}).Where("id = ?", b.ID).UpdateColumn("name", "~"+b.ID).Error; err != nil {
				return err
			}
		}
	}
	for position, b := range req.Bindings {
		next, err := rowOf(req, b, position)
		if err != nil {
			return err
		}
		row, exists := stored[b.ID]
		switch {
		case !exists:
			next.Revision = 1
			next.CreatedBy, next.CreatedAt = req.Actor, req.Now
			if err := tx.Create(&next).Error; err != nil {
				return err
			}
			if err := tx.Create(revisionOf(next, req)).Error; err != nil {
				return err
			}
		case sameContent(row, next):
			if err := tx.Model(&schema.AgentTool{}).Where("id = ?", b.ID).Updates(map[string]interface{}{"position": position, "name": row.Name}).Error; err != nil {
				return err
			}
		default:
			next.Revision = row.Revision + 1
			if err := tx.Model(&schema.AgentTool{}).Where("id = ?", b.ID).Updates(map[string]interface{}{
				"name": next.Name, "description": next.Description, "config": next.Config, "visibility": next.Visibility,
				"position": position, "revision": next.Revision, "target_key": next.TargetKey,
				"updated_by": req.Actor, "updated_at": req.Now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(revisionOf(next, req)).Error; err != nil {
				return err
			}
		}
	}
	return mirrorLegacyBindings(tx, req.AgentID, req.Bindings)
}

func rowOf(req SyncRequest, b agent.ToolBinding, position int) (schema.AgentTool, error) {
	config, err := encodeJSON(b.Config, "{}")
	if err != nil {
		return schema.AgentTool{}, err
	}
	visibility, err := encodeJSON(b.Visibility, "[]")
	if err != nil {
		return schema.AgentTool{}, err
	}
	testInputs, err := encodeJSON(b.TestInputs, "[]")
	if err != nil {
		return schema.AgentTool{}, err
	}
	return schema.AgentTool{
		ID: b.ID, WorkspaceID: req.WorkspaceID, AgentID: req.AgentID, Type: b.Name, Name: b.Function,
		Description: b.Description, Config: config, Visibility: visibility, TestInputs: testInputs, Position: position,
		TargetKey: b.TargetKey, UpdatedBy: req.Actor, UpdatedAt: req.Now,
	}, nil
}

func revisionOf(row schema.AgentTool, req SyncRequest) *schema.AgentToolRevision {
	return &schema.AgentToolRevision{
		ToolID: row.ID, Revision: row.Revision, WorkspaceID: row.WorkspaceID, AgentID: row.AgentID, Type: row.Type,
		Name: row.Name, Description: row.Description, Config: row.Config, Visibility: row.Visibility,
		SavedBy: req.Actor, SavedAt: req.Now,
	}
}

func sameContent(stored, next schema.AgentTool) bool {
	return stored.Name == next.Name && stored.Description == next.Description && stored.TargetKey == next.TargetKey &&
		sameJSON(stored.Config, next.Config) && sameJSON(stored.Visibility, next.Visibility)
}

func sameJSON(a, b string) bool {
	var left, right interface{}
	if json.Unmarshal([]byte(a), &left) != nil || json.Unmarshal([]byte(b), &right) != nil {
		return a == b
	}
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return bytes.Equal(l, r)
}

func encodeJSON(v interface{}, empty string) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(raw) == "null" {
		return empty, nil
	}
	return string(raw), nil
}

func bindingOf(row schema.AgentTool) agent.ToolBinding {
	b := agent.ToolBinding{
		ID: row.ID, Name: row.Type, Function: row.Name, Description: row.Description,
		Revision: row.Revision, TargetKey: row.TargetKey,
	}
	var config map[string]interface{}
	if json.Unmarshal([]byte(row.Config), &config) == nil && len(config) > 0 {
		b.Config = config
	}
	var visibility []agent.ToolVisibility
	if json.Unmarshal([]byte(row.Visibility), &visibility) == nil && len(visibility) > 0 {
		b.Visibility = visibility
	}
	var inputs []testinput.TestInput
	if json.Unmarshal([]byte(row.TestInputs), &inputs) == nil && len(inputs) > 0 {
		b.TestInputs = inputs
	}
	return b
}

func SaveTestInputs(db *gorm.DB, agentID, toolID string, inputs []testinput.TestInput) error {
	raw, err := encodeJSON(inputs, "[]")
	if err != nil {
		return err
	}
	result := db.Model(&schema.AgentTool{}).Where("id = ? AND agent_id = ?", toolID, agentID).UpdateColumn("test_inputs", raw)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	return nil
}
