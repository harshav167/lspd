package policy

// ResetSession clears remembered diagnostics for a session.
func (e *Engine) ResetSession(sessionID string) {
	e.mu.Lock()
	delete(e.seen, sessionID)
	e.mu.Unlock()
}
