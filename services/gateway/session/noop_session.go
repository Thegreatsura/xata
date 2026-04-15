package session

import "context"

type noopSession struct{}

var _noopSession = noopSession{}

func NoopSession() Session {
	return _noopSession
}

func (noopSession) BranchID() string {
	return ""
}

func (noopSession) ServeSQLSession(ctx context.Context) error {
	return nil
}
