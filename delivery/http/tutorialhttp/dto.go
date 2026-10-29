package tutorialhttp

import (
	"time"

	"vozko/domain/tutorial"
)

type ProgressRequest struct {
	Status *string `json:"status" example:"completed" enums:"in_progress,completed,dismissed"`
	Step   int     `json:"step" example:"3" minimum:"0" maximum:"50"`
}

type ProgressResponse struct {
	TutorialID string    `json:"tutorialId" example:"sites.editor"`
	Status     string    `json:"status" example:"completed" enums:"in_progress,completed,dismissed"`
	Step       int       `json:"step" example:"3"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type ProgressListResponse struct {
	Items []ProgressResponse `json:"items"`
}

type ChecklistItemResponse struct {
	ID   string `json:"id" example:"publish"`
	Done bool   `json:"done"`
}

type ChecklistResponse struct {
	Area  string                  `json:"area" example:"sites"`
	Items []ChecklistItemResponse `json:"items"`
	Done  int                     `json:"done" example:"2"`
	Total int                     `json:"total" example:"7"`
}

func presentProgress(p tutorial.Progress) ProgressResponse {
	return ProgressResponse{TutorialID: p.TutorialID, Status: string(p.Status), Step: p.Step, UpdatedAt: p.UpdatedAt}
}

func presentProgressList(list []tutorial.Progress) ProgressListResponse {
	items := make([]ProgressResponse, len(list))
	for i, p := range list {
		items[i] = presentProgress(p)
	}
	return ProgressListResponse{Items: items}
}

func presentChecklist(c tutorial.Checklist) ChecklistResponse {
	items := make([]ChecklistItemResponse, len(c.Items))
	for i, item := range c.Items {
		items[i] = ChecklistItemResponse{ID: item.ID, Done: item.Done}
	}
	return ChecklistResponse{Area: string(c.Area), Items: items, Done: c.DoneCount(), Total: len(items)}
}
