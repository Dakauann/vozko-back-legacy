package conversation

import "slices"

type Origin string

const (
	OriginReceptive Origin = "receptive"
	OriginCampaign  Origin = "campaign"
	OriginOutreach  Origin = "outreach"
	OriginUnknown   Origin = "unknown"
)

func Origins() []Origin {
	return []Origin{OriginReceptive, OriginCampaign, OriginOutreach, OriginUnknown}
}

func (o Origin) Valid() bool {
	return slices.Contains(Origins(), o)
}

func ParseOrigin(raw string) (Origin, error) {
	origin := Origin(raw)
	if !origin.Valid() {
		return "", ErrOriginInvalid
	}
	return origin, nil
}

func OriginSenders(origin Origin) []SenderKind {
	switch origin {
	case OriginReceptive:
		return []SenderKind{SenderContact}
	case OriginCampaign:
		return []SenderKind{SenderCampaign}
	case OriginOutreach:
		return ReplySenderKinds()
	}
	return nil
}

func OriginOf(kind SenderKind) Origin {
	for _, origin := range Origins() {
		if slices.Contains(OriginSenders(origin), kind) {
			return origin
		}
	}
	return OriginUnknown
}

func NonOpeningSenderKinds() []SenderKind {
	return []SenderKind{SenderSystem}
}

func (k SenderKind) OpensConversation() bool {
	return !slices.Contains(NonOpeningSenderKinds(), k)
}
