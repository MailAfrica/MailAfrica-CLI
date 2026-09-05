// Package output renders command results: tables for humans, indented JSON for
// scripts. Every renderer takes an io.Writer (commands pass cmd.OutOrStdout())
// so output is captured correctly in tests and scripts.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// JSON writes an indented JSON document to w.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table renders headers + rows as aligned columns to w.
func Table(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

// Bool renders a boolean as yes/no.
func Bool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Empty renders a placeholder for nil/empty display values.
func Empty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// SecretShort renders a redacted preview of a secret, e.g. MAIL_ab12… — used
// where showing only an identifying prefix is safe.
func SecretShort(s string) string {
	if len(s) <= 10 {
		return Empty(s)
	}
	return s[:8] + "…"
}
