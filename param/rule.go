package param

import (
	"fmt"
	"strings"
)

type rule interface {
	apply(Values) *Error
	names() []string
	ruleSpec() RuleSpec
}

type presenceRule struct {
	paramNames []string
	minPresent int
	maxPresent int
	param      string
}

// ExactlyOne requires callers to provide exactly one parameter from names.
func ExactlyOne(names ...string) presenceRule {
	return presenceRule{paramNames: names, minPresent: 1, maxPresent: 1}
}

// AtLeastOne requires callers to provide one or more parameters from names.
func AtLeastOne(names ...string) presenceRule {
	return presenceRule{paramNames: names, minPresent: 1}
}

// AtMostOne allows callers to provide zero or one parameter from names.
func AtMostOne(names ...string) presenceRule {
	return presenceRule{paramNames: names, maxPresent: 1}
}

func (rule presenceRule) apply(values Values) *Error {
	visible := rule.visibleNames(values)
	if len(visible) == 0 {
		return nil
	}

	minPresent := min(rule.minPresent, len(visible))

	present := make([]string, 0, len(visible))
	for _, name := range visible {
		if values.Present(name) {
			present = append(present, name)
		}
	}

	if minPresent > 0 && len(present) < minPresent {
		return paramError(
			rule.errorParam(values.path, visible),
			CodeRequiredChoice,
			rule.missingMessage(visible, minPresent),
			nil,
		)
	}
	if rule.maxPresent > 0 && len(present) > rule.maxPresent {
		return paramError(
			rule.errorParam(values.path, visible),
			CodeMutuallyExclusive,
			rule.conflictMessage(visible, present),
			nil,
		)
	}
	return nil
}

func (rule presenceRule) names() []string {
	return rule.paramNames
}

func (rule presenceRule) ruleSpec() RuleSpec {
	rule = normalizePresenceRule(rule)
	names := make([]string, len(rule.paramNames))
	copy(names, rule.paramNames)
	return RuleSpec{
		Names:      names,
		MinPresent: rule.minPresent,
		MaxPresent: rule.maxPresent,
	}
}

func (rule presenceRule) visibleNames(values Values) []string {
	visible := make([]string, 0, len(rule.paramNames))
	for _, name := range rule.paramNames {
		if _, ok := values.params[name]; ok {
			visible = append(visible, name)
		}
	}
	return visible
}

func normalizePresenceRule(rule presenceRule) presenceRule {
	rule.paramNames = normalizeNames("presence rule parameter", rule.paramNames)
	if rule.minPresent < 0 {
		panic("httpapi/param: presence rule minimum cannot be negative")
	}
	if rule.maxPresent < 0 {
		panic("httpapi/param: presence rule maximum cannot be negative")
	}
	if rule.minPresent == 0 && rule.maxPresent == 0 {
		panic("httpapi/param: presence rule requires a minimum or maximum")
	}
	if rule.minPresent > 0 && rule.maxPresent > 0 && rule.minPresent > rule.maxPresent {
		panic("httpapi/param: presence rule minimum cannot exceed maximum")
	}
	if rule.minPresent > len(rule.paramNames) {
		panic("httpapi/param: presence rule minimum cannot exceed parameter count")
	}
	rule.param = strings.TrimSpace(rule.param)
	return rule
}

func (rule presenceRule) errorParam(parent string, names []string) string {
	if rule.param != "" {
		return joinPath(parent, rule.param)
	}
	if parent == "" {
		return strings.Join(names, "|")
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, joinPath(parent, name))
	}
	return strings.Join(paths, "|")
}

func (rule presenceRule) missingMessage(names []string, minPresent int) string {
	if minPresent == 1 {
		return fmt.Sprintf("one of %s is required", formatNameList(names))
	}
	return fmt.Sprintf(
		"at least %d of %s are required",
		minPresent,
		formatNameList(names),
	)
}

func (rule presenceRule) conflictMessage(names []string, present []string) string {
	if rule.maxPresent == 1 {
		return fmt.Sprintf("%s are mutually exclusive; provide only one", formatNameList(names))
	}
	return fmt.Sprintf(
		"at most %d of %s may be provided; got %s",
		rule.maxPresent,
		formatNameList(names),
		formatNameList(present),
	)
}

func formatNameList(names []string) string {
	switch len(names) {
	case 0:
		return "no parameters"
	case 1:
		return fmt.Sprintf("`%s`", names[0])
	case 2:
		return fmt.Sprintf("`%s` or `%s`", names[0], names[1])
	default:
		quoted := make([]string, 0, len(names))
		for _, name := range names {
			quoted = append(quoted, fmt.Sprintf("`%s`", name))
		}
		return strings.Join(quoted[:len(quoted)-1], ", ") + ", or " + quoted[len(quoted)-1]
	}
}
