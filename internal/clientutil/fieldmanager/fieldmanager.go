package fieldmanager

import (
	"fmt"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// FieldManagerName returns the unified field manager name for a Hive controller.
// This replaces the inconsistent versioned prefixes (hive1-, hive2-, hive4-, hive5-, hive6-, hive7-)
// used in legacy code with a consistent "hive-{controllername}" format.
//
// Format: "hive-{controllername}"
// Example: FieldManagerName("clustersync") returns "hive-clustersync"
func FieldManagerName(controllerName hivev1.ControllerName) string {
	return "hive-" + string(controllerName)
}

// FieldManagerNameLegacy returns a legacy field manager name with a version prefix.
// This is provided for backward compatibility during migration but should not be used
// in new code. The legacy versioning scheme caused field ownership conflicts.
//
// Deprecated: Use FieldManagerName() instead. This function exists only to support
// migration scenarios where controllers must temporarily use old field manager names.
// Remove usage after migration completes.
//
// Legacy versions:
//   - v1: "hive1-{controller}" - Used by controller/utils
//   - v2: "hive2-{controller}" - Used by remoteclient
//   - v4: "hive4-{controller}" - Used by resource helper Create
//   - v5: "hive5-{controller}" - Used by resource helper CreateOrUpdate
//   - v6: "hive6-{controller}" - Used by resource helper Apply
//   - v7: "hive7-{controller}" - Used by resource helper Patch
func FieldManagerNameLegacy(controllerName hivev1.ControllerName, version int) string {
	return fmt.Sprintf("hive%d-%s", version, string(controllerName))
}
