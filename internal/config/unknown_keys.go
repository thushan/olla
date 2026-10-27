package config

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxKeyWalkDepth bounds the node walk. Decoding has already succeeded by the
// time we get here, so this is only a guard against pathological alias chains.
const maxKeyWalkDepth = 64

var yamlUnmarshalerType = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()

// UnknownKey is a key present in the config file that no Config field reads.
type UnknownKey struct {
	Path string // full dotted path, e.g. "proxy.retry"
	Line int
}

func (k UnknownKey) String() string {
	return fmt.Sprintf("%s (line %d)", k.Path, k.Line)
}

// FindUnknownKeys reports every key in the YAML document that Config does not
// model. yaml.v3's KnownFields(true) only yields a type name and line per
// error, with no full path, so this walks the node tree against the struct
// instead. Free-form maps are never reported; their keys are user data.
// It never fails: input that does not decode simply yields nothing.
func FindUnknownKeys(data []byte) []UnknownKey {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []UnknownKey
	walkKnownKeys(&doc, reflect.TypeOf(Config{}), "", 0, &out)
	return out
}

func walkKnownKeys(n *yaml.Node, t reflect.Type, path string, depth int, out *[]UnknownKey) {
	if n == nil || depth > maxKeyWalkDepth {
		return
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// Custom unmarshalers own their own schema.
	if t.Implements(yamlUnmarshalerType) || reflect.PointerTo(t).Implements(yamlUnmarshalerType) {
		return
	}

	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			walkKnownKeys(c, t, path, depth+1, out)
		}
	case yaml.AliasNode:
		walkKnownKeys(n.Alias, t, path, depth+1, out)
	case yaml.SequenceNode:
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			for i, c := range n.Content {
				walkKnownKeys(c, t.Elem(), fmt.Sprintf("%s[%d]", path, i), depth+1, out)
			}
		}
	case yaml.MappingNode:
		switch t.Kind() {
		case reflect.Struct:
			walkStruct(n, t, path, depth, out)
		case reflect.Map:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if isMergeKey(n.Content[i]) {
					continue
				}
				walkKnownKeys(n.Content[i+1], t.Elem(), joinKeyPath(path, n.Content[i].Value), depth+1, out)
			}
		default:
			// Scalars and interfaces in this position have no keys to check.
		}
	default:
		// Scalar and null nodes carry no keys.
	}
}

func walkStruct(n *yaml.Node, t reflect.Type, path string, depth int, out *[]UnknownKey) {
	fields, freeForm := structKeys(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if isMergeKey(k) {
			walkMerged(v, t, path, depth+1, out)
			continue
		}
		ft, ok := fields[k.Value]
		if !ok {
			// Top-level "x-" keys are the compose-style extension convention,
			// used purely to hold anchors. Content merged from them into a real
			// section is still checked there.
			if path == "" && strings.HasPrefix(k.Value, "x-") {
				continue
			}
			if !freeForm {
				*out = append(*out, UnknownKey{Path: joinKeyPath(path, k.Value), Line: k.Line})
			}
			continue
		}
		walkKnownKeys(v, ft, joinKeyPath(path, k.Value), depth+1, out)
	}
}

// walkMerged applies a `<<` merge value (mapping, alias, or sequence of them)
// as if its entries were written inline in the parent mapping.
func walkMerged(v *yaml.Node, t reflect.Type, path string, depth int, out *[]UnknownKey) {
	if depth > maxKeyWalkDepth {
		return
	}
	switch v.Kind {
	case yaml.AliasNode:
		walkMerged(v.Alias, t, path, depth+1, out)
	case yaml.SequenceNode:
		for _, c := range v.Content {
			walkMerged(c, t, path, depth+1, out)
		}
	case yaml.MappingNode:
		walkStruct(v, t, path, depth+1, out)
	default:
		// A non-mapping merge value is a decode error, which already failed Load.
	}
}

func isMergeKey(k *yaml.Node) bool {
	return k.Kind == yaml.ScalarNode && k.Tag == "!!merge"
}

func joinKeyPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// structKeys maps yaml key names to field types for t, flattening ",inline"
// structs. freeForm is true when an inline map accepts arbitrary keys.
func structKeys(t reflect.Type) (fields map[string]reflect.Type, freeForm bool) {
	fields = make(map[string]reflect.Type, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if strings.Contains(","+opts+",", ",inline,") {
			ft := f.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Struct:
				sub, sf := structKeys(ft)
				for k, v := range sub {
					fields[k] = v
				}
				freeForm = freeForm || sf
			case reflect.Map:
				freeForm = true
			default:
				// yaml.v3 rejects other inline types at decode time.
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		fields[name] = f.Type
	}
	return fields, freeForm
}
