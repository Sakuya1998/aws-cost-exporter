package costexplorer

import "strings"

// isThrottleCode reports whether an AWS error code indicates request throttling.
func isThrottleCode(code string) bool {
	code = strings.ToLower(code)
	return strings.Contains(code, "thrott") ||
		code == "limitexceededexception" ||
		code == "toomanyrequestsexception" ||
		code == "requestlimitexceeded"
}
