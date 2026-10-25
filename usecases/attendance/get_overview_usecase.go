package attendance_usecase

import (
	"context"
	"errors"
	"math"
	"time"

	"vozko/domain/agent_presence"
	"vozko/domain/attendance"
	"vozko/domain/cache"
	"vozko/domain/callsession"
	"vozko/domain/queue_event"
	"vozko/domain/shared/deps"
)

type QueueStats interface {
	Stats(workspaceID string, from, to *time.Time) (*queue_event.Stats, error)
}

type OccupancyReader interface {
	Occupancy(workspaceID string, from, to *time.Time) ([]agent_presence.OccupancyRow, error)
}

type PresenceLister interface {
	ListPresence(workspaceID string) []callsession.MemberPresence
}

type getOverviewUseCase struct {
	repo      attendance.Repository
	queue     QueueStats
	presence  OccupancyReader
	live      PresenceLister
	schedules *ScheduleResolver
	targets   *TargetsService
	memo      cache.Memo
	gate      cache.Gate
	versions  cache.Versions
	ttl       time.Duration
	now       func() time.Time
}

type OverviewService interface {
	attendance.GetOverviewUseCase
	attendance.OverviewSectionsUseCase
}

type OverviewDeps struct {
	Repo      attendance.Repository
	Queue     QueueStats
	Presence  OccupancyReader
	Live      PresenceLister
	Schedules *ScheduleResolver
	Targets   *TargetsService
	Caching   SectionCaching
	Clock     func() time.Time
}

var ErrSectionTTLRequired = errors.New("attendance overview: cached sections need a positive TTL")

func NewGetOverviewUseCase(d OverviewDeps) (OverviewService, error) {
	if err := deps.Require("attendance overview", map[string]any{
		"overview repository": d.Repo,
		"queue stats":         d.Queue,
		"occupancy":           d.Presence,
		"live presence":       d.Live,
		"schedules":           d.Schedules,
		"targets":             d.Targets,
		"section memo":        d.Caching.Memo,
		"analytics gate":      d.Caching.Gate,
		"section versions":    d.Caching.Versions,
	}); err != nil {
		return nil, err
	}
	if d.Caching.TTL <= 0 {
		return nil, ErrSectionTTLRequired
	}
	if d.Clock == nil {
		d.Clock = nowUTC
	}
	return &getOverviewUseCase{
		repo:      d.Repo,
		queue:     d.Queue,
		presence:  d.Presence,
		live:      d.Live,
		schedules: d.Schedules,
		targets:   d.Targets,
		memo:      d.Caching.Memo,
		gate:      d.Caching.Gate,
		versions:  d.Caching.Versions,
		ttl:       d.Caching.TTL,
		now:       d.Clock,
	}, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func (uc *getOverviewUseCase) Execute(workspaceID string, filter attendance.OverviewFilter) (*attendance.Overview, error) {
	var out *attendance.Overview
	err := uc.shareScope(context.Background(), func(ctx context.Context) error {
		summary, err := uc.summaryNow(ctx, workspaceID, filter)
		if err != nil {
			return err
		}
		trend, err := uc.trendFrom(ctx, workspaceID, filter, summary)
		if err != nil {
			return err
		}
		team, err := uc.teamFrom(ctx, workspaceID, filter, summary)
		if err != nil {
			return err
		}
		stages, err := uc.stagesNow(ctx, workspaceID, filter)
		if err != nil {
			return err
		}
		backlog, err := uc.backlogNow(ctx, workspaceID, filter)
		if err != nil {
			return err
		}
		rework, err := uc.reworkNow(ctx, workspaceID, filter)
		if err != nil {
			return err
		}
		out = &attendance.Overview{
			SummarySection: *summary,
			TeamSection:    *team,
			TrendSection:   *trend,
			StagesSection:  *stages,
			BacklogSection: *backlog,
			ReworkSection:  *rework,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out.LiveSection = uc.liveSection(workspaceID, filter)
	return out, nil
}

func (uc *getOverviewUseCase) clock() time.Time {
	return uc.now()
}

func (uc *getOverviewUseCase) liveSection(workspaceID string, filter attendance.OverviewFilter) attendance.LiveSection {
	return attendance.LiveSection{
		Queue:     uc.queueStats(workspaceID, filter),
		Occupancy: uc.occupancy(workspaceID, filter),
		Live:      uc.livePresence(workspaceID),
	}
}

func (uc *getOverviewUseCase) queueStats(workspaceID string, filter attendance.OverviewFilter) attendance.OverviewQueue {
	st, err := uc.queue.Stats(workspaceID, filter.DateFrom, filter.DateTo)
	if err != nil || st == nil {
		return attendance.OverviewQueue{Available: false}
	}
	out := attendance.OverviewQueue{
		Enqueued:    st.Enqueued,
		Connected:   st.Connected,
		Abandoned:   st.Abandoned,
		Overflow:    st.Overflow,
		QueueFull:   st.QueueFull,
		Cancelled:   st.Cancelled,
		AbandonRate: st.AbandonRate,
		Available:   st.Enqueued > 0 || st.Connected > 0 || st.Abandoned > 0,
	}
	if st.Connected > 0 && st.AvgASAMs > 0 {
		mins := math.Round((st.AvgASAMs/60000)*100) / 100
		out.AvgASAMins = &mins
	}
	return out
}

func (uc *getOverviewUseCase) occupancy(workspaceID string, filter attendance.OverviewFilter) attendance.OverviewOccupancy {
	rows, err := uc.presence.Occupancy(workspaceID, filter.DateFrom, filter.DateTo)
	if err != nil || len(rows) == 0 {
		return attendance.OverviewOccupancy{Available: false}
	}
	var onlineMS, onCallMS int64
	var sumPct float64
	var n int64
	for _, row := range rows {
		onlineMS += row.OnlineMS
		onCallMS += row.OnCallMS
		if row.OnlineMS > 0 {
			sumPct += row.Occupancy
			n++
		}
	}
	out := attendance.OverviewOccupancy{
		AgentsSampled: int64(len(rows)),
		OnlineMS:      onlineMS,
		OnCallMS:      onCallMS,
		Available:     onlineMS > 0 || onCallMS > 0,
	}
	if n > 0 {
		avg := math.Round(sumPct/float64(n)*100) / 100
		out.AvgOccupancyPct = &avg
	}
	if onlineMS > 0 {
		team := math.Round(float64(onCallMS)/float64(onlineMS)*10000) / 100
		out.TeamOccupancyPct = &team
		idle := max(math.Round((100-team)*100)/100, 0)
		out.TeamIdlePct = &idle
	}
	return out
}

func (uc *getOverviewUseCase) livePresence(workspaceID string) attendance.OverviewLive {
	if workspaceID == "" {
		return attendance.OverviewLive{HasData: false, AsOf: uc.clock()}
	}
	rows := uc.live.ListPresence(workspaceID)
	live := attendance.OverviewLive{
		HasData: true,
		AsOf:    uc.clock(),
		Agents:  make([]attendance.OverviewLiveAgent, 0, len(rows)),
	}
	for _, p := range rows {
		live.Online++
		if p.Busy {
			live.InCall++
		} else {
			live.Free++
		}
		live.Agents = append(live.Agents, attendance.OverviewLiveAgent{
			UserID:     p.UserID,
			Busy:       p.Busy,
			HasBrowser: p.HasBrowser,
		})
	}
	if live.Online > 0 {
		idle := math.Round(float64(live.Free)/float64(live.Online)*10000) / 100
		busy := math.Round(float64(live.InCall)/float64(live.Online)*10000) / 100
		live.IdleRatePct = &idle
		live.BusyRatePct = &busy
	}
	return live
}
