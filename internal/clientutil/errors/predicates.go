package errors

// This file previously contained error predicate wrappers (IsNotFound, IsAlreadyExists, etc.)
// that had zero production usage. Controllers prefer direct usage of apierrors.* functions.
// The predicates were removed as part of unused code cleanup.
