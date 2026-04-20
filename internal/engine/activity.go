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

func (e *Engine) GetAllLogs(ctx context.Context, limit int) (interface{}, error) {
	if limit == 0 {
		limit = 50
	}
	return e.db.ListAllOperations(limit)
}
