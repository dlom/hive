package errors

import (
	"context"
	stderr "errors"
	"net"
	"net/url"
	"syscall"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsNotFound returns true if the error indicates a resource was not found.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsNotFound(err)
}

// IsAlreadyExists returns true if the error indicates a resource already exists.
func IsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsAlreadyExists(err)
}

// IsConflict returns true if the error indicates an update conflict (optimistic concurrency failure).
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsConflict(err)
}

// IsTimeout returns true if the error indicates a timeout occurred.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}

	// Check for context deadline exceeded
	if stderr.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for Kubernetes timeout errors
	if apierrors.IsTimeout(err) {
		return true
	}

	// Check for network timeout errors
	var netErr net.Error
	if stderr.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check for URL timeout errors
	var urlErr *url.Error
	if stderr.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}

	return false
}

// IsConnectionFailed returns true if the error indicates a network connection failure.
func IsConnectionFailed(err error) bool {
	if err == nil {
		return false
	}

	// Check for connection refused
	if stderr.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	// Check for connection reset
	if stderr.Is(err, syscall.ECONNRESET) {
		return true
	}

	// Check for network unreachable
	if stderr.Is(err, syscall.ENETUNREACH) {
		return true
	}

	// Check for host unreachable
	if stderr.Is(err, syscall.EHOSTUNREACH) {
		return true
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if stderr.As(err, &dnsErr) {
		return true
	}

	// Check for generic network errors
	var opErr *net.OpError
	if stderr.As(err, &opErr) {
		// OpError can wrap many types - check if it's connection-related
		if opErr.Op == "dial" || opErr.Op == "connect" {
			return true
		}
	}

	return false
}

// IsAuthenticationFailed returns true if the error indicates an authentication or authorization failure.
func IsAuthenticationFailed(err error) bool {
	if err == nil {
		return false
	}

	// Check for Kubernetes unauthorized/forbidden errors
	if apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err) {
		return true
	}

	return false
}

// IsInvalidResource returns true if the error indicates resource validation failed.
func IsInvalidResource(err error) bool {
	if err == nil {
		return false
	}

	// Check for Kubernetes invalid/bad request errors
	if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
		return true
	}

	return false
}

// IsCanceled returns true if the error indicates the context was canceled.
func IsCanceled(err error) bool {
	if err == nil {
		return false
	}
	return stderr.Is(err, context.Canceled)
}
