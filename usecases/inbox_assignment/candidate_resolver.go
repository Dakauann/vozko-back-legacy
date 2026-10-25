package inbox_assignment_usecase

import (
	"log"
	"math/rand/v2"
	"time"

	conversation "vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared/deps"
	wsc "vozko/domain/workspace_config"
)

const (
	PoolReasonOnline        = "online"
	PoolReasonLastSeen      = "last_seen"
	PoolReasonLastSeenEmpty = "last_seen_empty_fallback"
	PoolReasonLastSeenError = "last_seen_error_fallback"

	PoolReasonNoRecipientAvailable = "no_recipient_available"
	PoolReasonAtCapacity           = "everyone_at_capacity"
	PoolReasonWorkloadUnavailable  = "workload_unavailable"
	PoolReasonHandoutsUnavailable  = "handouts_unavailable"
)

type Pool struct {
	Ring     []string
	Rotation ia.Rotation
	Mode     string
	Strategy ia.Strategy
	Reason   string
}

func (p Pool) CanPick() bool { return p.Rotation.CanPick() }

type availability struct {
	cands  []ia.Candidate
	resume ia.ResumePolicy
	mode   string
	reason string
}

type CandidateResolver struct {
	online   conversation.EligibleUserProvider
	roster   ia.RosterProvider
	seen     ia.LastSeenReader
	workload ia.WorkloadReader
	handouts ia.HandoutReader
	now      func() time.Time
	draw     func(n int) int
}

type CandidateDeps struct {
	Online   conversation.EligibleUserProvider
	Roster   ia.RosterProvider
	Presence ia.LastSeenReader
	Workload ia.WorkloadReader
	Handouts ia.HandoutReader
	Now      func() time.Time
	Draw     func(n int) int
}

func NewCandidateResolver(d CandidateDeps) (*CandidateResolver, error) {
	if err := deps.Require("roulette candidates", map[string]any{
		"online":   d.Online,
		"roster":   d.Roster,
		"presence": d.Presence,
		"workload": d.Workload,
		"handouts": d.Handouts,
	}); err != nil {
		return nil, err
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Draw == nil {
		d.Draw = rand.IntN
	}
	return &CandidateResolver{online: d.Online, roster: d.Roster, seen: d.Presence, workload: d.Workload, handouts: d.Handouts, now: d.Now, draw: d.Draw}, nil
}

func (r *CandidateResolver) Resolve(workspaceID, departmentID string, skipAdmins bool, cfg *wsc.WorkspaceConfig) Pool {
	return r.arrange(workspaceID, departmentID, cfg, r.available(workspaceID, departmentID, skipAdmins, cfg))
}

func (r *CandidateResolver) arrange(workspaceID, departmentID string, cfg *wsc.WorkspaceConfig, avail availability) Pool {
	strategy := cfg.EffectiveRouletteStrategy()
	pool := Pool{Mode: avail.mode, Strategy: strategy, Reason: avail.reason}

	cands := avail.cands
	if strategy.ChoosesFromRecipients() {
		cands = ia.OnlyRecipients(cands, cfg.EffectiveRouletteRecipients())
		if len(cands) == 0 && len(avail.cands) > 0 {
			log.Printf("[InboxAssignment] workspace %s department %q: none of the %s recipients is available", workspaceID, departmentID, strategy)
			pool.Reason = PoolReasonNoRecipientAvailable
			return pool
		}
	}

	cands, failure := r.withReadings(workspaceID, departmentID, strategy, cfg.EffectiveRouletteMaxOpenPerMember(), cands)
	if failure != "" {
		pool.Reason = failure
		return pool
	}

	pool.Rotation = ia.Arrange(strategy, cands, avail.resume, r.draw)
	pool.Ring = pool.Rotation.Order
	return pool
}

func (r *CandidateResolver) withReadings(workspaceID, departmentID string, strategy ia.Strategy, maxOpen int, cands []ia.Candidate) ([]ia.Candidate, string) {
	if len(cands) == 0 {
		return cands, ""
	}
	ids := ia.CandidateIDs(cands)

	if strategy.RanksByWorkload() || maxOpen > 0 {
		open, err := r.workload.OpenConversations(workspaceID, ids)
		if err != nil {
			log.Printf("[InboxAssignment] workspace %s open-conversation counts failed: %v; assigning nobody", workspaceID, err)
			return nil, PoolReasonWorkloadUnavailable
		}
		for i := range cands {
			cands[i].OpenConversations = open[cands[i].UserID]
		}
		cands = ia.WithinCapacity(cands, maxOpen)
		if len(cands) == 0 {
			log.Printf("[InboxAssignment] workspace %s department %q: everyone holds %d open conversation(s) or more", workspaceID, departmentID, maxOpen)
			return nil, PoolReasonAtCapacity
		}
	}

	if strategy.RanksByHandouts() {
		last, err := r.handouts.LastHandouts(workspaceID, ia.CandidateIDs(cands))
		if err != nil {
			log.Printf("[InboxAssignment] workspace %s hand-out times failed: %v; assigning nobody", workspaceID, err)
			return nil, PoolReasonHandoutsUnavailable
		}
		for i := range cands {
			cands[i].LastHandout = last[cands[i].UserID]
		}
	}
	return cands, ""
}

func (r *CandidateResolver) available(workspaceID, departmentID string, skipAdmins bool, cfg *wsc.WorkspaceConfig) availability {
	mode := cfg.EffectiveRouletteMode()
	if mode != wsc.RouletteModeLastSeen {
		return r.onlinePool(workspaceID, departmentID, skipAdmins, PoolReasonOnline)
	}

	members, err := r.roster.ListRouletteMembers(workspaceID, departmentID, skipAdmins)
	if err != nil {
		log.Printf("[InboxAssignment] roster error for workspace %s department %q: %v; falling back to the online pool", workspaceID, departmentID, err)
		return r.onlinePool(workspaceID, departmentID, skipAdmins, PoolReasonLastSeenError)
	}
	if len(members) == 0 {
		log.Printf("[InboxAssignment] last_seen pool empty for workspace %s department %q (roster=0); falling back to the online pool", workspaceID, departmentID)
		return r.onlinePool(workspaceID, departmentID, skipAdmins, PoolReasonLastSeenEmpty)
	}

	lastSeen, err := r.seen.LastSeen(workspaceID, members)
	if err != nil {
		log.Printf("[InboxAssignment] presence read error for workspace %s: %v; falling back to the online pool", workspaceID, err)
		return r.onlinePool(workspaceID, departmentID, skipAdmins, PoolReasonLastSeenError)
	}

	onlineNow := make(map[string]bool)
	for _, uid := range r.onlineUsers(workspaceID, departmentID, skipAdmins) {
		onlineNow[uid] = true
	}

	now := r.now().UTC()
	window := cfg.EffectiveRouletteLastSeenWindow()

	cands := make([]ia.Candidate, 0, len(members))
	byID := make(map[string]ia.Candidate, len(members))
	for _, uid := range members {
		c := ia.Candidate{UserID: uid, Online: onlineNow[uid]}
		if c.Online {
			c.LastSeen = now
		} else if seenAt, ok := lastSeen[uid]; ok {
			if seenAt.After(now) {
				seenAt = now
			}
			c.LastSeen = seenAt
		}
		cands = append(cands, c)
		byID[uid] = c
	}

	ring := ia.BuildLastSeenRing(cands, now, window)
	if len(ring) == 0 {
		log.Printf("[InboxAssignment] last_seen pool empty for workspace %s department %q (roster=%d, window=%s, nobody seen inside it); falling back to the online pool",
			workspaceID, departmentID, len(members), window)
		return r.onlinePool(workspaceID, departmentID, skipAdmins, PoolReasonLastSeenEmpty)
	}

	ordered := make([]ia.Candidate, len(ring))
	for i, uid := range ring {
		ordered[i] = byID[uid]
	}
	return availability{cands: ordered, resume: ia.ResumeAtHead, mode: wsc.RouletteModeLastSeen, reason: PoolReasonLastSeen}
}

func (r *CandidateResolver) onlinePool(workspaceID, departmentID string, skipAdmins bool, reason string) availability {
	ring := ia.BuildOnlineRing(r.onlineUsers(workspaceID, departmentID, skipAdmins))
	cands := make([]ia.Candidate, len(ring))
	for i, uid := range ring {
		cands[i] = ia.Candidate{UserID: uid, Online: true}
	}
	return availability{cands: cands, resume: ia.ResumeAtInsertionPoint, mode: wsc.RouletteModeOnline, reason: reason}
}

func (r *CandidateResolver) onlineUsers(workspaceID, departmentID string, skipAdmins bool) []string {
	if r.online == nil {
		return nil
	}
	if departmentID != "" {
		return r.online.GetEligibleUsersForWorkspaceDepartment(workspaceID, departmentID, skipAdmins)
	}
	return r.online.GetEligibleUsersForWorkspace(workspaceID, skipAdmins)
}
