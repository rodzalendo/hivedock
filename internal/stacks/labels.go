package stacks

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeLabels accepts either compose label form:
//
//	labels:                 labels:
//	  key: value      OR      - key=value
//
// and normalizes both to a map.
type composeLabels map[string]string

func (l *composeLabels) UnmarshalYAML(node *yaml.Node) error {
	out := map[string]string{}
	switch node.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := node.Decode(&m); err != nil {
			return err
		}
		for k, v := range m {
			out[k] = v
		}
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return err
		}
		for _, item := range items {
			k, v, _ := strings.Cut(item, "=")
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	case 0:
		// null / absent
	default:
		return fmt.Errorf("labels: unexpected yaml node kind %d", node.Kind)
	}
	*l = out
	return nil
}
