package api

import (
	"log/slog"
	"strings"

	"connectrpc.com/connect/v2"
)

func internalError(operation string, err error) error {
	slog.Error("request failed", "operation", operation, "error", err)
	return connect.NewError(connect.CodeInternal, "internal error")
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, message)
}

// placementFailedPrecondition maps scheduler / runtime placement errors to
// FailedPrecondition so clients see the real reason instead of "internal error".
// Returns nil when err is not a placement failure.
func placementFailedPrecondition(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "unschedulable") ||
		strings.Contains(msg, "no feasible") ||
		strings.Contains(msg, "no online") ||
		strings.Contains(msg, "no runtime") ||
		strings.Contains(msg, "no nodes enrolled") {
		return connect.NewError(connect.CodeFailedPrecondition, msg)
	}
	return nil
}
