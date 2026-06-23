// Package output renders command results as human tables or JSON (with optional jq).
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/itchyny/gojq"
)

// Printer carries the global output preferences.
type Printer struct {
	JSON bool
	JQ   string
}

// Success prints a green-ish success line to stderr.
func Success(msg string) { fmt.Fprintln(os.Stderr, "✓ "+msg) }

// Info prints an informational line to stderr.
func Info(msg string) { fmt.Fprintln(os.Stderr, msg) }

// Errf prints an error line to stderr.
func Errf(format string, a ...any) { fmt.Fprintf(os.Stderr, "✗ "+format+"\n", a...) }

// Emit prints v as JSON when --json/--jq is set; otherwise calls human().
func (p *Printer) Emit(v any, human func()) error {
	if p.JSON || p.JQ != "" {
		return p.emitJSON(v)
	}
	human()
	return nil
}

func (p *Printer) emitJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if p.JQ == "" {
		var pretty any
		_ = json.Unmarshal(raw, &pretty)
		out, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	query, err := gojq.Parse(p.JQ)
	if err != nil {
		return fmt.Errorf("invalid jq: %w", err)
	}
	var input any
	_ = json.Unmarshal(raw, &input)
	iter := query.Run(input)
	for {
		got, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := got.(error); ok {
			return err
		}
		out, _ := json.MarshalIndent(got, "", "  ")
		fmt.Println(string(out))
	}
	return nil
}

// Table writes a simple aligned table to stdout.
func Table(headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}
