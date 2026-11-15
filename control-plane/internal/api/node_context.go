package api

import (
	"context"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

type nodeContextKey struct{}

func contextWithNode(ctx context.Context, node db.Node) context.Context {
	return context.WithValue(ctx, nodeContextKey{}, node)
}

func NodeFromContext(ctx context.Context) (db.Node, bool) {
	node, ok := ctx.Value(nodeContextKey{}).(db.Node)
	return node, ok
}
