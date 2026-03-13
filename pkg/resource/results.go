package resource

type ApplyState int

const (
	Created ApplyState = iota
	Configured
	Unchanged
)

func (s ApplyState) String() string {
	switch s {
	case Created:
		return "created"
	case Configured:
		return "configured"
	case Unchanged:
		return "unchanged"
	default:
		return "unknown"
	}
}

type PatchState int

const (
	Patched PatchState = iota
	PatchUnchanged
)

func (s PatchState) String() string {
	switch s {
	case Patched:
		return "patched"
	case PatchUnchanged:
		return "unchanged"
	default:
		return "unknown"
	}
}

type DeleteState int

const (
	Deleted DeleteState = iota
	NotFound
	DeletionInProgress
)

func (s DeleteState) String() string {
	switch s {
	case Deleted:
		return "deleted"
	case NotFound:
		return "not-found"
	case DeletionInProgress:
		return "deletion-in-progress"
	default:
		return "unknown"
	}
}
