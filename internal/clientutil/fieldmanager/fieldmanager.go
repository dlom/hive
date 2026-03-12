package fieldmanager

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// FieldManagerName returns the unified field manager name for a Hive controller.
//
// Format: "hive-{controllername}"
// Example: FieldManagerName("clustersync") returns "hive-clustersync"
//
// Historical note:
// This replaces an inconsistent versioned naming scheme that caused field ownership conflicts
// in Server-Side Apply. The legacy scheme used different version prefixes depending on which
// code path created the field manager:
//   - v1: "hive1-{controller}" - controller/utils
//   - v2: "hive2-{controller}" - remoteclient
//   - v4: "hive4-{controller}" - resource helper Create
//   - v5: "hive5-{controller}" - resource helper CreateOrUpdate
//   - v6: "hive6-{controller}" - resource helper Apply
//   - v7: "hive7-{controller}" - resource helper Patch
//
// The unified "hive-{controller}" format ensures all operations from the same controller
// use the same field manager name, preventing ownership conflicts.
func FieldManagerName(controllerName hivev1.ControllerName) string {
	return "hive-" + string(controllerName)
}
