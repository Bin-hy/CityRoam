package domain

import "context"

type RouteProvider interface {
	Route(ctx context.Context, from, to POI, mode string) (Transport, error)
}
