package githistory

// manager implements HistoryManager interface by combining reader and modifier
type manager struct {
	HistoryReader
	HistoryModifier
}

// NewManager creates a new HistoryManager instance that combines reader and modifier
func NewManager(runner Runner) HistoryManager {
	return &manager{
		HistoryReader:   NewReader(runner),
		HistoryModifier: NewModifier(runner),
	}
}

// New is a convenience function that creates a complete HistoryManager
func New(runner Runner) HistoryManager {
	return NewManager(runner)
}