package adapter

// OpenCodeAdapter is the MVP implementation of AgentAdapter for OpenCode.
// TODO (M2): implement each method against the OpenCode API. Keep every
// OpenCode-specific detail inside this file, per DESIGN.md.
type OpenCodeAdapter struct {
	// TODO: connection details, process handle, etc.
}

// NewOpenCodeAdapter constructs an OpenCodeAdapter. TODO (M2): fill in.
func NewOpenCodeAdapter() *OpenCodeAdapter {
	return &OpenCodeAdapter{}
}

var _ AgentAdapter = (*OpenCodeAdapter)(nil)

func (a *OpenCodeAdapter) Start(projectDir string) (string, error) {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) Stop(sessionID string) error {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) Restart(sessionID string) error {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) Status(sessionID string) (Status, error) {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) SendMessage(sessionID string, message string) error {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) Abort(sessionID string) error {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) RespondToPermission(sessionID string, requestID string, approved bool) error {
	panic("not implemented, see M2 in WORKFLOW.md")
}

func (a *OpenCodeAdapter) GetEvents(sessionID string) ([]Event, error) {
	panic("not implemented, see M2 in WORKFLOW.md")
}
