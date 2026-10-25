package attendance_usecase

import (
	"context"
	"time"

	"vozko/domain/attendance"
	"vozko/domain/cache"
)

const sectionCachePrefix = "attendance:overview"

type SectionCaching struct {
	Memo     cache.Memo
	Gate     cache.Gate
	Versions cache.Versions
	TTL      time.Duration
}

func (uc *getOverviewUseCase) Summary(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.SummarySection, error) {
	return rememberSection(ctx, uc, workspaceID, attendance.SectionSummary, filter,
		func(ctx context.Context) (*attendance.SummarySection, error) {
			return uc.summaryNow(ctx, workspaceID, filter)
		})
}

func (uc *getOverviewUseCase) Trend(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.TrendSection, error) {
	return rememberSection(ctx, uc, workspaceID, attendance.SectionTrend, filter,
		func(ctx context.Context) (*attendance.TrendSection, error) {
			var out *attendance.TrendSection
			err := uc.shareScope(ctx, func(ctx context.Context) error {
				summary, err := uc.Summary(ctx, workspaceID, filter)
				if err != nil {
					return err
				}
				out, err = uc.trendFrom(ctx, workspaceID, filter, summary)
				return err
			})
			return out, err
		})
}

func (uc *getOverviewUseCase) Team(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.TeamSection, error) {
	return rememberSection(ctx, uc, workspaceID, attendance.SectionTeam, filter,
		func(ctx context.Context) (*attendance.TeamSection, error) {
			var out *attendance.TeamSection
			err := uc.shareScope(ctx, func(ctx context.Context) error {
				summary, err := uc.Summary(ctx, workspaceID, filter)
				if err != nil {
					return err
				}
				out, err = uc.teamFrom(ctx, workspaceID, filter, summary)
				return err
			})
			return out, err
		})
}

func (uc *getOverviewUseCase) Stages(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.StagesSection, error) {
	return rememberSection(ctx, uc, workspaceID, attendance.SectionStages, filter,
		func(ctx context.Context) (*attendance.StagesSection, error) {
			return uc.stagesNow(ctx, workspaceID, filter)
		})
}

func (uc *getOverviewUseCase) Backlog(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.BacklogSection, error) {
	return rememberSection(ctx, uc, workspaceID, attendance.SectionBacklog, filter,
		func(ctx context.Context) (*attendance.BacklogSection, error) {
			return uc.backlogNow(ctx, workspaceID, filter)
		})
}

func (uc *getOverviewUseCase) Rework(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.ReworkSection, error) {
	return rememberSection(ctx, uc, workspaceID, attendance.SectionRework, filter,
		func(ctx context.Context) (*attendance.ReworkSection, error) {
			return uc.reworkNow(ctx, workspaceID, filter)
		})
}

func (uc *getOverviewUseCase) summaryNow(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.SummarySection, error) {
	var out *attendance.SummarySection
	err := uc.gated(ctx, func(ctx context.Context) error {
		var err error
		out, err = uc.computeSummary(ctx, workspaceID, filter)
		return err
	})
	return out, err
}

func (uc *getOverviewUseCase) trendFrom(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
	summary *attendance.SummarySection,
) (*attendance.TrendSection, error) {
	out := &attendance.TrendSection{}
	err := uc.gated(ctx, func(ctx context.Context) error {
		window, err := uc.loadWindow(ctx, workspaceID, filter)
		if err != nil {
			return err
		}
		out.Trend, err = uc.buildTrend(ctx, workspaceID, filter, window, summary)
		return err
	})
	return out, err
}

func (uc *getOverviewUseCase) teamFrom(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
	summary *attendance.SummarySection,
) (*attendance.TeamSection, error) {
	var out *attendance.TeamSection
	err := uc.gated(ctx, func(ctx context.Context) error {
		window, err := uc.loadWindow(ctx, workspaceID, filter)
		if err != nil {
			return err
		}
		if out, err = uc.repo.ReadTeam(ctx, workspaceID, filter); err != nil {
			return err
		}
		out.TeamRanking = uc.buildTeamRanking(workspaceID, filter, window, summary, out.ByMember)
		return nil
	})
	return out, err
}

func (uc *getOverviewUseCase) stagesNow(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.StagesSection, error) {
	out := &attendance.StagesSection{}
	err := uc.gated(ctx, func(ctx context.Context) error {
		var err error
		out.Stages, err = uc.repo.ReadStages(ctx, workspaceID, filter)
		return err
	})
	return out, err
}

func (uc *getOverviewUseCase) backlogNow(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.BacklogSection, error) {
	out := &attendance.BacklogSection{}
	err := uc.gated(ctx, func(ctx context.Context) error {
		var err error
		out.BacklogXray, err = uc.repo.ReadBacklog(ctx, workspaceID, filter, uc.clock())
		return err
	})
	return out, err
}

func (uc *getOverviewUseCase) reworkNow(
	ctx context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.ReworkSection, error) {
	out := &attendance.ReworkSection{}
	err := uc.gated(ctx, func(ctx context.Context) error {
		var err error
		out.Rework, err = uc.repo.ReadRework(ctx, workspaceID, filter)
		return err
	})
	return out, err
}

func rememberSection[T any](
	ctx context.Context,
	uc *getOverviewUseCase,
	workspaceID string,
	section attendance.Section,
	filter attendance.OverviewFilter,
	compute func(context.Context) (T, error),
) (T, error) {
	version, err := uc.versions.Version(workspaceID)
	if err != nil {
		return compute(ctx)
	}
	key := sectionCachePrefix + ":" + workspaceID + ":" + version + ":" + filter.Fingerprint(section)
	return cache.Remember(ctx, uc.memo, key, uc.ttl, compute)
}

func (uc *getOverviewUseCase) Live(
	_ context.Context,
	workspaceID string,
	filter attendance.OverviewFilter,
) (*attendance.LiveSection, error) {
	out := uc.liveSection(workspaceID, filter)
	return &out, nil
}
