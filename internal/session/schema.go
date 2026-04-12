// Package session provides session data schema and YAML persistence.
package session

// Session represents the per-agent session state stored in a temporary data file.
type Session struct {
	SnoozedPipelines []string       `yaml:"snoozed_pipelines"`
	LastTick         *LastTickState `yaml:"last_tick"`
	// SlowCheckWarnedScopes tracks scopes that have already surfaced the
	// slow automatic-invariant warning during this session.
	SlowCheckWarnedScopes []string `yaml:"slow_check_warned_scopes,omitempty"`
}

// LastTickState records the last tick context for change detection.
type LastTickState struct {
	Pipeline  string `yaml:"pipeline"`
	Job       string `yaml:"job"`
	Timestamp string `yaml:"timestamp"`
}
