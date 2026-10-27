package security

import "net/http"

// RejectFunc writes a security rejection for routes whose clients expect a specific
// error format (the translator routes answer in the translator's own error schema).
// Headers such as Retry-After are already set on w when it is called. It is an alias
// so the router can name the same type without importing this package.
type RejectFunc = func(w http.ResponseWriter, status int, message string)

// writeReject sends a rejection through reject, or as plain text when reject is nil.
func writeReject(w http.ResponseWriter, reject RejectFunc, status int, message string) {
	if reject == nil {
		http.Error(w, message, status)
		return
	}
	reject(w, status, message)
}
