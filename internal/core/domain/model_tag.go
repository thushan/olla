package domain

import "strings"

// OllamaDefaultTag is the tag Ollama assumes when a model name carries none:
// "ollama pull foo" lists as "foo:latest" and clients still ask for "foo".
const OllamaDefaultTag = "latest"

// OllamaDefaultTagName returns name with Ollama's default tag appended and true
// when name carries no tag, false otherwise.
//
// A colon only starts a tag when no path separator follows it: the colon in
// "registry.example.com:5000/foo" belongs to the host, so that name is untagged.
// The toggle is one-way ("foo" to "foo:latest", never back): Ollama always lists
// models tagged, so a native bare "foo" cannot come from Ollama.
// Callers must apply this after an exact lookup misses and only to
// Ollama-sourced entries. Stripping ":latest" the other way, or applying this to
// other platforms, would hand a backend a name it does not serve.
func OllamaDefaultTagName(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if i := strings.LastIndexByte(name, ':'); i >= 0 && !strings.Contains(name[i+1:], "/") {
		return "", false
	}
	return name + ":" + OllamaDefaultTag, true
}
