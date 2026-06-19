package engine

import "context"

func (e *Engine) LogActivity(domain, action, details, result string) {
	e.db.LogOperation(domain, action, details, result)
}

func (e *Engine) GetLogs(ctx context.Context, domain string, limit int) (interface{}, error) {
	if limit == 0 {
		limit = 50
	}
	return e.db.ListOperations(domain, limit)
}

// GetAllLogs returns recent activity. A non-empty owner restricts it to that
// owner's sites; an empty owner (admin) returns everything.
func (e *Engine) GetAllLogs(ctx context.Context, owner string, limit int) (interface{}, error) {
	if limit == 0 {
		limit = 50
	}
	if owner == "" {
		return e.db.ListAllOperations(limit)
	}
	return e.db.ListOperationsByOwner(owner, limit)
}
