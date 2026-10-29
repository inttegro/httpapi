package param

import callerpkg "github.com/inttegro/httpapi/caller"

func availabilityAllows(availability callerpkg.Set, caller callerpkg.Caller) bool {
	return availability.Allows(caller)
}
