package resource

import (
	"k8s.io/apimachinery/pkg/types"
)

type patchOptions struct {
	fieldManager string
	patchType    types.PatchType
}

type PatchOption func(*patchOptions)

func WithPatchType(pt types.PatchType) PatchOption {
	return func(opts *patchOptions) {
		opts.patchType = pt
	}
}
