package lint

type Severity string

const (
	SeverityBlocking Severity = "blocking"
	SeverityAdvisory Severity = "advisory"
	SeverityInfo     Severity = "info"
)
