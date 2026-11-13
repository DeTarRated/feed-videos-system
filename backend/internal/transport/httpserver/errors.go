package httpserver

import "strings"

func isPublishUnavailableError(err error) bool {
	return strings.Contains(err.Error(), "publisher") ||
		strings.Contains(err.Error(), "confirm") ||
		strings.Contains(err.Error(), "rabbitmq") ||
		strings.Contains(err.Error(), "unavailable")
}
