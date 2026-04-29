package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Response is the versioned output envelope for JSON mode.
type Response struct {
	Version int `json:"version"`
	Data    any `json:"data"`
}

// ErrorResponse is the versioned error envelope for JSON mode.
type ErrorResponse struct {
	Version int         `json:"version"`
	Error   ErrorDetail `json:"error"`
}

// ErrorDetail holds structured error information.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// PrintData outputs data in the given format.
func PrintData(format string, data any) error {
	return FprintData(os.Stdout, format, data)
}

// FprintData writes data to a writer in the given format.
func FprintData(w io.Writer, format string, data any) error {
	switch format {
	case "json":
		return printJSON(w, &Response{Version: 1, Data: data})
	case "yaml":
		return printYAML(w, data)
	case "table":
		return printTable(w, data)
	default:
		return printJSON(w, &Response{Version: 1, Data: data})
	}
}

// PrintError writes a structured error to stderr.
func PrintError(format, code, message string, details any) {
	if format == "json" {
		resp := &ErrorResponse{
			Version: 1,
			Error: ErrorDetail{
				Code:    code,
				Message: message,
				Details: details,
			},
		}
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Fprintln(os.Stderr, string(data))
	} else {
		fmt.Fprintf(os.Stderr, "Error [%s]: %s\n", code, message)
	}
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printYAML(w io.Writer, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func printTable(w io.Writer, data any) error {
	switch v := data.(type) {
	case []map[string]any:
		return printRecordsTable(w, v)
	case map[string]any:
		return printRecordTable(w, v)
	default:
		// Fall back to YAML for types we can't table-ify
		return printYAML(w, data)
	}
}

func printRecordsTable(w io.Writer, records []map[string]any) error {
	if len(records) == 0 {
		fmt.Fprintln(w, "(no records)")
		return nil
	}

	// Collect all keys from first record
	var keys []string
	for k := range records[0] {
		keys = append(keys, k)
	}

	// Header
	fmt.Fprintln(w, strings.Join(keys, "\t"))
	fmt.Fprintln(w, strings.Repeat("-", len(keys)*16))

	// Rows
	for _, rec := range records {
		var vals []string
		for _, k := range keys {
			vals = append(vals, fmt.Sprintf("%v", rec[k]))
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}

	return nil
}

func printRecordTable(w io.Writer, record map[string]any) error {
	for k, v := range record {
		fmt.Fprintf(w, "%-20s %v\n", k+":", v)
	}
	return nil
}
