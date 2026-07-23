package awsx

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return strings.Contains(code, "NotFound") || strings.Contains(code, "InvalidGroup.NotFound") ||
			strings.Contains(code, "InvalidInstanceID.NotFound") || strings.Contains(code, "InvalidAllocationID.NotFound") ||
			strings.Contains(code, "InvalidAddress.NotFound")
	}

	return false
}

func isIncorrectInstanceStateForAlreadyStopped(err error) bool {
	if err == nil {
		return false
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	code := apiErr.ErrorCode()
	msg := apiErr.ErrorMessage()
	return strings.Contains(code, "IncorrectInstanceState") && strings.Contains(strings.ToLower(msg), "stopped")
}
