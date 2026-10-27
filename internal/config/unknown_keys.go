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

// maxKeyWalkVisits caps total node visits. Merge sequences can reference an
// anchor repeatedly (even cyclically, when an explicit key overrides the merge
// at decode time), so depth alone does not bound the work.
const maxKeyWalkVisits = 100_000

type keyWalk struct {
	out    []UnknownKey
	budget int
}

// step spends one visit and reports whether the walk may continue.
func (w *keyWalk) step() bool {
	w.budget--
	return w.budget >= 0
}

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
	w := &keyWalk{budget: maxKeyWalkVisits}
	walkKnownKeys(&doc, reflect.TypeOf(Config{}), "", 0, w)
	return w.out
}

func walkKnownKeys(n *yaml.Node, t reflect.Type, path string, depth int, w *keyWalk) {
	if n == nil || depth > maxKeyWalkDepth || !w.step() {
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
			walkKnownKeys(c, t, path, depth+1, w)
		}
	case yaml.AliasNode:
		walkKnownKeys(n.Alias, t, path, depth+1, w)
	case yaml.SequenceNode:
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			for i, c := range n.Content {
				walkKnownKeys(c, t.Elem(), fmt.Sprintf("%s[%d]", path, i), depth+1, w)
			}
		}
	case yaml.MappingNode:
		switch t.Kind() {
		case reflect.Struct:
			walkStruct(n, t, path, depth, w)
		case reflect.Map:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if isMergeKey(n.Content[i]) {
					continue
				}
				walkKnownKeys(n.Content[i+1], t.Elem(), joinKeyPath(path, n.Content[i].Value), depth+1, w)
			}
		default:
			// Scalars and interfaces in this position have no keys to check.
		}
	default:
		// Scalar and null nodes carry no keys.
	}
}

func walkStruct(n *yaml.Node, t reflect.Type, path string, depth int, w *keyWalk) {
	fields, freeForm := structKeys(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		// Each entry costs a visit, so reports are bounded by the same budget.
		if !w.step() {
			return
		}
		k, v := n.Content[i], n.Content[i+1]
		// yaml.v3 only honours a merge written literally as `<<`, not an alias to one.
		if isMergeKey(k) {
			walkMerged(v, t, path, depth+1, w)
			continue
		}
		keyLine := k.Line
		// A key written as an alias (*anchor) resolves to the anchored scalar.
		if k.Kind == yaml.AliasNode && k.Alias != nil {
			k = k.Alias
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
				w.out = append(w.out, UnknownKey{Path: joinKeyPath(path, k.Value), Line: keyLine})
			}
			continue
		}
		walkKnownKeys(v, ft, joinKeyPath(path, k.Value), depth+1, w)
	}
}

// walkMerged applies a `<<` merge value (mapping, alias, or sequence of them)
// as if its entries were written inline in the parent mapping.
func walkMerged(v *yaml.Node, t reflect.Type, path string, depth int, w *keyWalk) {
	if depth > maxKeyWalkDepth || !w.step() {
		return
	}
	switch v.Kind {
	case yaml.AliasNode:
		walkMerged(v.Alias, t, path, depth+1, w)
	case yaml.SequenceNode:
		for _, c := range v.Content {
			walkMerged(c, t, path, depth+1, w)
		}
	case yaml.MappingNode:
		walkStruct(v, t, path, depth+1, w)
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
