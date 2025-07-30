package ui

// PRConfig holds PR configuration
type PRConfig struct {
	CreatePR    bool
	Remote      string
	Base        string
	Draft       bool
	Provider    string
	UseTemplate bool // Whether to use template
}

// Message type definitions
type delayedPushMsg struct{}
type delayedCreatePRMsg struct{}
type startCommitPhaseMsg struct{}

type prPreviewReadyMsg struct {
	data PRPreviewData
}

type createPRDoneMsg struct {
	err   error
	prURL string
}

// Decision represents the user's choice in the Review UI
type Decision int

const (
	DecisionNone Decision = iota
	DecisionAccept
	DecisionCancel
)

// ReviewModel is used to display the commit message for user confirmation/editing.
// When the user presses a/e/c, the program ends and returns the decision and final message.
// For user-friendliness, supports up/down key to switch buttons (simplified implementation).
