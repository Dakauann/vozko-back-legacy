package container

import (
	"context"
	"log"
	"strings"
	"time"

	attendancehttp "vozko/delivery/http/attendance"
	domainCache "vozko/domain/cache"
	"vozko/domain/conversation"
	wsc "vozko/domain/workspace_config"
	redisCache "vozko/infra/cache"
	attendance_usecase "vozko/usecases/attendance"
	conversation_usecase "vozko/usecases/conversation"
)

type outcomeCaptureReader struct {
	configs wsc.Repository
}

func (r outcomeCaptureReader) OutcomeCaptureFor(ctx context.Context, workspaceID string) (*conversation.OutcomeCapture, error) {
	if r.configs == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	config, err := r.configs.GetByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}
	return config.OutcomeCapture, nil
}

var _ conversation_usecase.OutcomeCaptureReader = outcomeCaptureReader{}

type entryDepartmentLookup interface {
	GetEntryDepartmentID(entryID, entryType string) (string, error)
}

type entryDepartmentResolver struct {
	lookup entryDepartmentLookup
}

func (r entryDepartmentResolver) DepartmentIDForEntry(ctx context.Context, entryID, entryType string) (string, error) {
	if r.lookup == nil {
		return "", nil
	}
	return r.lookup.GetEntryDepartmentID(entryID, entryType)
}

var _ conversation_usecase.EntryDepartmentResolver = entryDepartmentResolver{}

func (c *Container) attendanceScheduleResolver() *attendance_usecase.ScheduleResolver {
	return attendance_usecase.NewScheduleResolver(
		c.repositories.workspaceConfig,
		c.repositories.workspaceDepartment,
	)
}

const (
	analyticsGateCapacity       = 3
	analyticsGateMaxWait        = 20 * time.Second
	analyticsComputeTimeout     = 30 * time.Second
	attendanceSectionTTL        = 60 * time.Second
	attendanceSectionVersionKey = "attendance:overview"
)

func (c *Container) sharedAnalyticsGate() domainCache.Gate {
	if c.analyticsGate == nil {
		c.analyticsGate = redisCache.NewSemaphoreGate(analyticsGateCapacity, analyticsGateMaxWait)
	}
	return c.analyticsGate
}

func (c *Container) analyticsMemo() domainCache.Memo {
	return redisCache.NewSharedStateMemo(c.redisProvider.SharedState(), analyticsComputeTimeout)
}

func (c *Container) attendanceSectionVersions() domainCache.Versions {
	return redisCache.NewSharedStateVersions(c.redisProvider.SharedState(), attendanceSectionVersionKey)
}

func (c *Container) attendanceSectionCaching() attendance_usecase.SectionCaching {
	return attendance_usecase.SectionCaching{
		Memo:     c.analyticsMemo(),
		Gate:     c.sharedAnalyticsGate(),
		Versions: c.attendanceSectionVersions(),
		TTL:      attendanceSectionTTL,
	}
}

func (c *Container) attendanceTargetsService() *attendance_usecase.TargetsService {
	if c.attendanceTargets != nil {
		return c.attendanceTargets
	}
	service, err := attendance_usecase.NewTargetsService(attendance_usecase.TargetsDeps{
		Repo:      c.repositories.attendanceTarget,
		Schedules: c.attendanceScheduleResolver(),
		Versions:  c.attendanceSectionVersions(),
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.attendanceTargets = service
	return service
}

func (c *Container) attendanceOverview() attendance_usecase.OverviewService {
	overview, err := attendance_usecase.NewGetOverviewUseCase(attendance_usecase.OverviewDeps{
		Repo:      c.repositories.attendance,
		Queue:     c.repositories.queueEvent,
		Presence:  c.repositories.agentPresence,
		Live:      c.services.callSessions,
		Schedules: c.attendanceScheduleResolver(),
		Targets:   c.attendanceTargetsService(),
		Caching:   c.attendanceSectionCaching(),
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	return overview
}

func (c *Container) entryDepartmentResolver() entryDepartmentResolver {
	lookup, ok := c.services.campaignWorkspaceResolver.(entryDepartmentLookup)
	if !ok {
		log.Fatalf("[conversation-status] the workspace resolver cannot name the department of a conversation, close outcomes per department cannot be enforced")
	}
	return entryDepartmentResolver{lookup: lookup}
}

func (c *Container) memberActivityUseCase() *attendance_usecase.MemberActivityUseCase {
	return attendance_usecase.NewMemberActivityUseCase(attendance_usecase.MemberActivityDeps{
		Presence:    c.repositories.agentPresence,
		History:     c.repositories.assignmentHistory,
		Departments: c.repositories.workspaceDepartment,
		Team:        c.useCases.getOverview,
		Schedules:   c.attendanceScheduleResolver(),
	})
}

func (c *Container) attendanceHandler() *attendancehttp.AttendanceHandler {
	handler, err := attendancehttp.NewAttendanceHandler(attendancehttp.Deps{
		AttendanceStats: c.useCases.getAttendanceStats,
		WindowStats:     c.useCases.getWindowStats,
		ResponseTimes:   c.useCases.getResponseTimeDistribution,
		AIAgentStats:    c.useCases.getAIAgentStats,
		FRTStats:        c.useCases.getFRTStats,
		Sections:        c.useCases.getOverview,
		Queue:           c.repositories.queueEvent,
		Presence:        c.repositories.agentPresence,
		Targets:         c.attendanceTargetsService(),
		TargetScoper:    c.services.conversationAuthImpl,
		MemberActivity:  c.memberActivityUseCase(),
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	return handler
}
