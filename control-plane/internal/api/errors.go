package api

import (
	"log/slog"

	"connectrpc.com/connect/v2"
)

func internalError(operation string, err error) error {
	slog.Error("request failed", "operation", operation, "error", err)
	return connect.NewError(connect.CodeInternal, "internal error")
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, message)
}
