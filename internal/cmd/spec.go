package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// marshalSpecYAML renders v as 2-space YAML, matching the gitops repo convention.
func marshalSpecYAML(v any) ([]byte, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return []byte(b.String()), nil
}

// yamlUnmarshal parses YAML/JSON bytes into v.
func yamlUnmarshal(data []byte, v any) error {
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse spec: %w", err)
	}
	return nil
}

// loadSpecFile unmarshals a YAML or JSON spec file into v (YAML is a JSON superset).
// "-" reads from stdin.
func loadSpecFile(path string, v any) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// editInEditor opens $EDITOR on the given content and returns the saved bytes,
// or (nil, nil) if the user left it unchanged.
func editInEditor(initial []byte, suffix string) ([]byte, error) {
	f, err := os.CreateTemp("", "naru-*."+suffix)
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(initial); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor) // allow "code -w" etc.
	ed := exec.Command(parts[0], append(parts[1:], name)...)
	ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := ed.Run(); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}

	edited, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if string(edited) == string(initial) {
		return nil, nil
	}
	return edited, nil
}
