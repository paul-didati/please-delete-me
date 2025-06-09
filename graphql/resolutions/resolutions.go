package resolutions

import "context"

type Resolver interface {
	Resolve(ctx context.Context, entity, field string, args, zero any, object, list, nullable bool) (any, error)
}
