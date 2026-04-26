package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sergio-bershadsky/secondbrain-db/internal/events"
)

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Append-only event log operations",
	Long: `The events log is an append-only, immutable record of every state-changing
operation sbdb performs. See docs/EVENTS.md for the full spec.`,
}

var eventAppendCmd = &cobra.Command{
	Use:   "append",
	Short: "Append a single event to today's log",
	RunE:  runEventAppend,
}

var eventTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List every registered event type",
	RunE:  runEventTypes,
}

var eventShowCmd = &cobra.Command{
	Use:   "show [N]",
	Short: "Show the last N events (default 20)",
	RunE:  runEventShow,
	Args:  cobra.MaximumNArgs(1),
}

var eventRebuildRegistryCmd = &cobra.Command{
	Use:   "rebuild-registry",
	Short: "Rebuild internal/events/registry.yaml from the event log",
	RunE:  runEventRebuildRegistry,
}

var eventRepairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Repair a corrupted event file (requires --truncate-partial)",
	Long: `When a process crashes mid-append, the last line of an events file may
be incomplete. doctor detects this but never auto-truncates. Run this
command with --truncate-partial to remove the trailing partial line.`,
	RunE: runEventRepair,
}

var (
	flagEventType  string
	flagEventID    string
	flagEventSHA   string
	flagEventPrev  string
	flagEventOp    string
	flagEventPhase string
	flagEventActor string
	flagEventData  string
	flagEventFile  string

	flagRepairTruncatePartial bool
)

func init() {
	eventAppendCmd.Flags().StringVar(&flagEventType, "type", "", "event type (e.g. note.created)")
	eventAppendCmd.Flags().StringVar(&flagEventID, "id", "", "entity id")
	eventAppendCmd.Flags().StringVar(&flagEventSHA, "sha", "", "post-state sha256 (hex)")
	eventAppendCmd.Flags().StringVar(&flagEventPrev, "prev", "", "pre-state sha256 (hex)")
	eventAppendCmd.Flags().StringVar(&flagEventOp, "op", "", "operation group ulid")
	eventAppendCmd.Flags().StringVar(&flagEventPhase, "phase", "", "sub-step within an op")
	eventAppendCmd.Flags().StringVar(&flagEventActor, "actor", "cli", "actor: cli|hook|worker|agent")
	eventAppendCmd.Flags().StringVar(&flagEventData, "data", "", "JSON payload (object) for data field")
	_ = eventAppendCmd.MarkFlagRequired("type")
	_ = eventAppendCmd.MarkFlagRequired("id")

	eventRepairCmd.Flags().StringVar(&flagEventFile, "file", "", "events file to repair (basename)")
	eventRepairCmd.Flags().BoolVar(&flagRepairTruncatePartial, "truncate-partial", false, "remove trailing partial line")

	eventCmd.AddCommand(eventAppendCmd)
	eventCmd.AddCommand(eventTypesCmd)
	eventCmd.AddCommand(eventShowCmd)
	eventCmd.AddCommand(eventRebuildRegistryCmd)
	eventCmd.AddCommand(eventRepairCmd)
	rootCmd.AddCommand(eventCmd)
}

func runEventAppend(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}

	registry, err := loadOrSeedRegistry(cfg.BasePath)
	if err != nil {
		return err
	}

	app := events.NewAppender(cfg.BasePath, cfg.Events.RotationLines)
	defer app.Close()
	em := events.NewEmitter(app, registry)

	ev := &events.Event{
		TS:    time.Now().UTC(),
		Type:  flagEventType,
		ID:    flagEventID,
		SHA:   flagEventSHA,
		Prev:  flagEventPrev,
		Op:    flagEventOp,
		Phase: flagEventPhase,
		Actor: events.Actor(flagEventActor),
	}
	if flagEventData != "" {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(flagEventData), &data); err != nil {
			return fmt.Errorf("invalid --data JSON: %w", err)
		}
		ev.Data = data
	}

	if err := em.Emit(context.Background(), ev); err != nil {
		return err
	}
	return nil
}

func runEventTypes(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	registry, err := loadOrSeedRegistry(cfg.BasePath)
	if err != nil {
		return err
	}

	buckets := make([]string, 0, len(registry.Buckets))
	for b := range registry.Buckets {
		buckets = append(buckets, b)
	}
	sort.Strings(buckets)
	for _, b := range buckets {
		entry := registry.Buckets[b]
		fmt.Fprintf(cmd.OutOrStdout(), "%s [owner=%s]\n", b, entry.Owner)
		for _, v := range entry.Types {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s.%s\n", b, v)
		}
		for _, v := range entry.Deprecated {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s.%s (deprecated)\n", b, v)
		}
	}
	return nil
}

func runEventShow(cmd *cobra.Command, args []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	limit := 20
	if len(args) == 1 {
		if _, err := fmt.Sscanf(args[0], "%d", &limit); err != nil {
			return fmt.Errorf("invalid limit %q: %w", args[0], err)
		}
	}

	var collected []*events.Event
	err = events.IterateLive(cfg.BasePath, func(_ string, e *events.Event) error {
		collected = append(collected, e)
		return nil
	})
	if err != nil {
		return err
	}
	if len(collected) > limit {
		collected = collected[len(collected)-limit:]
	}
	for _, e := range collected {
		line, err := e.MarshalLine()
		if err != nil {
			return err
		}
		_, _ = cmd.OutOrStdout().Write(line)
	}
	return nil
}

func runEventRebuildRegistry(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	registry, err := events.RebuildRegistry(cfg.BasePath)
	if err != nil {
		return err
	}
	path := cfg.BasePath + "/" + events.RegistryFileName
	if err := registry.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "registry rebuilt: %s\n", path)
	return nil
}

func runEventRepair(cmd *cobra.Command, _ []string) error {
	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	if !flagRepairTruncatePartial {
		return fmt.Errorf("repair requires --truncate-partial (explicit consent per spec §7.10)")
	}
	if flagEventFile == "" {
		return fmt.Errorf("--file is required")
	}
	if strings.Contains(flagEventFile, "/") || strings.Contains(flagEventFile, "..") {
		return fmt.Errorf("--file must be a basename, not a path")
	}
	path := cfg.BasePath + "/" + events.EventsDir + "/" + flagEventFile
	return truncatePartialLine(path)
}

// truncatePartialLine reads a file, finds the last newline, and truncates
// after it. Idempotent: if the file already ends in \n, it's a no-op.
func truncatePartialLine(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() == 0 {
		return nil
	}
	// Read backward to find last \n.
	const chunk = 4096
	pos := fi.Size()
	buf := make([]byte, chunk)
	lastNL := int64(-1)
	for pos > 0 {
		readSize := int64(chunk)
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		if _, err := f.ReadAt(buf[:readSize], pos); err != nil {
			return err
		}
		for i := readSize - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				lastNL = pos + i
				break
			}
		}
		if lastNL >= 0 {
			break
		}
	}
	if lastNL < 0 {
		// No newline anywhere; whole file is partial.
		return f.Truncate(0)
	}
	if lastNL == fi.Size()-1 {
		return nil // already clean
	}
	return f.Truncate(lastNL + 1)
}

// loadOrSeedRegistry loads registry.yaml if present, otherwise seeds with
// built-ins. The path is fixed at <basePath>/<events.RegistryFileName>.
func loadOrSeedRegistry(basePath string) (*events.Registry, error) {
	path := basePath + "/" + events.RegistryFileName
	if _, err := os.Stat(path); err == nil {
		return events.LoadRegistry(path)
	}
	return events.NewBuiltinRegistry(), nil
}
