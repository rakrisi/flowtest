package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rakrisi/flowtest/internal/config"
	"github.com/rakrisi/flowtest/internal/driver"
	"github.com/rakrisi/flowtest/internal/engine"
	"github.com/rakrisi/flowtest/internal/output"
	"github.com/spf13/cobra"
)

var version = "0.0.6"

func main() {
	rootCmd := &cobra.Command{
		Use:     "flowtest",
		Short:   "Declarative backend flow testing",
		Long:    "FlowTest — run YAML-driven backend integration test flows (API, DB, Kafka, Redis).",
		Version: version,
	}

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(initCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	var (
		profile    string
		verbose    bool
		failFast   bool
		jsonOut    bool
		htmlReport string
		startAt    int
		stepName   string
		fromJSON   string
		vars       []string
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "run <flow-file>",
		Short: "Execute a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowPath := args[0]

			// Load global config
			globalCfg, err := config.LoadGlobalConfig("flowtest.yaml")
			if err != nil {
				return err
			}

			// Load flow
			var flowCfg *config.FlowConfig
			if fromJSON != "" {
				flowCfg, err = config.LoadFlowFromJSON(fromJSON)
			} else {
				flowCfg, err = config.LoadFlowConfig(flowPath)
			}
			if err != nil {
				return err
			}

			// Merge environment
			env, err := config.MergeEnv(globalCfg, flowCfg, profile)
			if err != nil {
				return err
			}

			// Validate environment has required infra for this flow
			if err := config.ValidateEnv(flowCfg, &env); err != nil {
				return err
			}

			// Choose printer
			var printer engine.Printer
			if jsonOut {
				printer = &output.NullPrinter{}
			} else {
				printer = output.NewTerminalPrinter()
			}

			// Create engine
			eng := engine.NewEngine(printer, verbose, failFast)
			eng.SetDryRun(dryRun)

			// Register drivers (pass configured databases)
			registry, err := driver.NewRegistry(env.Databases)
			if err != nil {
				return fmt.Errorf("initializing drivers: %w", err)
			}
			defer registry.Close()
			registry.RegisterAll(eng)

			// Parse --var flags into initial variables
			initVars := parseVarFlags(vars)

			// Skip steps by index
			if startAt > 1 && startAt <= len(flowCfg.Steps) {
				flowCfg.Steps = flowCfg.Steps[startAt-1:]
			}

			// Skip steps by name — find the named step and start from there
			if stepName != "" {
				found := false
				for i, s := range flowCfg.Steps {
					if strings.EqualFold(s.Name, stepName) {
						flowCfg.Steps = flowCfg.Steps[i:]
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("step %q not found in flow. Available steps:\n%s", stepName, listStepNames(flowCfg))
				}
			}

			// Signal handling — Ctrl+C runs cleanup then exits
			sigCtx, sigCancel := context.WithCancel(context.Background())
			defer sigCancel()
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				select {
				case <-sigCh:
					fmt.Fprintf(os.Stderr, "\n  Interrupted — finishing current step and running cleanup...\n")
					sigCancel()
				case <-sigCtx.Done():
				}
			}()
			// Run
			result := eng.Run(sigCtx, flowCfg, &env, initVars)

			// Output
			if jsonOut {
				return output.PrintJSONResult(result)
			}

			output.PrintSummary(result)

			// Generate HTML report if requested
			if htmlReport != "" {
				if err := output.GenerateHTMLReport(result, output.HTMLReportConfig{
					OutputPath: htmlReport,
					Title:      "FlowTest Report",
					Subtitle:   flowCfg.Name,
				}); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to generate HTML report: %v\n", err)
				} else {
					fmt.Printf("\n  HTML report generated: %s\n", htmlReport)
				}
			}

			if !result.Success() {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&profile, "profile", "p", "", "Environment profile to use")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show request/response details, queries, assertions")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on first failure")
	cmd.Flags().BoolVar(&jsonOut, "output-json", false, "Output results as JSON")
	cmd.Flags().StringVar(&htmlReport, "html-report", "", "Generate HTML report to specified file (e.g., report.html)")
	cmd.Flags().StringVar(&fromJSON, "from-json", "", "Load flow definition from a JSON file instead of YAML")
	cmd.Flags().StringSliceVar(&vars, "var", nil, "Set initial variables (key=value), repeatable")
	cmd.Flags().IntVar(&startAt, "step", 0, "Start from step N (skip earlier steps)")
	cmd.Flags().StringVar(&stepName, "step-name", "", "Start from the named step")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would execute without running anything")

	return cmd
}

func listStepNames(cfg *config.FlowConfig) string {
	var lines []string
	for i, s := range cfg.Steps {
		lines = append(lines, fmt.Sprintf("  %d. %s", i+1, s.Name))
	}
	return strings.Join(lines, "\n")
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all flow files",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "flows"
			if len(args) > 0 {
				dir = args[0]
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No flows directory found. Run 'flowtest init' to get started.")
					return nil
				}
				return err
			}

			found := false
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
					flowPath := filepath.Join(dir, e.Name())
					flowCfg, err := config.LoadFlowConfig(flowPath)
					if err != nil {
						fmt.Printf("  %s  %s (parse error: %v)\n", "!", flowPath, err)
						continue
					}
					fmt.Printf("  %s  %s — %s\n", "·", flowPath, flowCfg.Name)
					found = true
				}
			}

			if !found {
				fmt.Println("No flow files found. Create .yaml files in the flows/ directory.")
			}
			return nil
		},
	}
}

func validateCmd() *cobra.Command {
	var checkConnections bool

	cmd := &cobra.Command{
		Use:   "validate <flow-file>",
		Short: "Validate a flow file without executing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowCfg, err := config.LoadFlowConfig(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Flow %q is valid.\n", flowCfg.Name)
			fmt.Printf("  Steps: %d\n", len(flowCfg.Steps))
			fmt.Printf("  Setup steps: %d\n", len(flowCfg.Setup))
			fmt.Printf("  Cleanup steps: %d\n", len(flowCfg.Cleanup))

			drivers := map[string]int{}
			for _, s := range flowCfg.Steps {
				drivers[s.DriverType()]++
			}
			fmt.Printf("  Drivers used:")
			for d, count := range drivers {
				fmt.Printf(" %s(%d)", d, count)
			}
			fmt.Println()

			if checkConnections {
				globalCfg, err := config.LoadGlobalConfig("flowtest.yaml")
				if err != nil {
					return err
				}
				profile, _ := cmd.Flags().GetString("profile")
				env, err := config.MergeEnv(globalCfg, flowCfg, profile)
				if err != nil {
					return err
				}

				fmt.Println("\n  Connection checks:")
				if env.APIBase != "" {
					checkTCP("  API", env.APIBase)
				}
				for name, dsn := range env.Databases {
					checkTCP(fmt.Sprintf("  DB[%s]", name), dsn)
				}
				if env.KafkaBrokers != "" {
					for _, broker := range strings.Split(env.KafkaBrokers, ",") {
						checkTCP("  Kafka", strings.TrimSpace(broker))
					}
				}
				if env.Redis != "" {
					checkTCP("  Redis", env.Redis)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&checkConnections, "check", false, "Check connectivity to configured services")
	cmd.Flags().StringP("profile", "p", "", "Environment profile to use")

	return cmd
}

func checkTCP(label string, addr string) {
	host := extractHost(addr)
	if host == "" {
		fmt.Printf("    %s: could not parse address %q\n", label, addr)
		return
	}

	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		fmt.Printf("    %s (%s): unreachable — %v\n", label, host, err)
		return
	}
	conn.Close()
	fmt.Printf("    %s (%s): ok\n", label, host)
}

func extractHost(addr string) string {
	if u, err := url.Parse(addr); err == nil && u.Host != "" {
		host := u.Host
		if !strings.Contains(host, ":") {
			switch u.Scheme {
			case "http":
				host += ":80"
			case "https":
				host += ":443"
			case "postgres", "postgresql":
				host += ":5432"
			case "mysql":
				host += ":3306"
			case "mongodb", "mongodb+srv":
				host += ":27017"
			case "redis", "rediss":
				host += ":6379"
			}
		}
		return host
	}
	if strings.Contains(addr, ":") {
		return addr
	}
	return ""
}

// service represents a backend service the user can test.
type service struct {
	key   string
	label string
	desc  string
}

var allServices = []service{
	{"http", "HTTP API", "REST endpoints (GET, POST, PUT, DELETE)"},
	{"postgres", "PostgreSQL", "SQL queries and data seeding"},
	{"mysql", "MySQL", "SQL queries with ? placeholders"},
	{"mongodb", "MongoDB", "Document operations (find, insert, update, delete)"},
	{"redis", "Redis", "Cache read/write, TTL, KEYS, HGETALL"},
	{"kafka", "Kafka", "Produce messages and consume/match events"},
}

func initCmd() *cobra.Command {
	var noInteractive bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new flowtest project",
		RunE: func(cmd *cobra.Command, args []string) error {
			var selected []string

			if noInteractive || !isTTY() {
				// Non-interactive: default to HTTP only
				selected = []string{"http"}
			} else {
				var err error
				selected, err = promptServices()
				if err != nil {
					return err
				}
			}

			globalCfg := buildGlobalConfig(selected)
			exampleFlow := buildExampleFlow(selected)

			// Write flowtest.yaml
			if _, err := os.Stat("flowtest.yaml"); os.IsNotExist(err) {
				if err := os.WriteFile("flowtest.yaml", []byte(globalCfg), 0644); err != nil {
					return fmt.Errorf("creating flowtest.yaml: %w", err)
				}
				fmt.Println("  Created flowtest.yaml")
			} else {
				fmt.Println("  flowtest.yaml already exists, skipping")
			}

			// Write flows/example.yaml
			if err := os.MkdirAll("flows", 0755); err != nil {
				return fmt.Errorf("creating flows directory: %w", err)
			}
			examplePath := "flows/example.yaml"
			if _, err := os.Stat(examplePath); os.IsNotExist(err) {
				if err := os.WriteFile(examplePath, []byte(exampleFlow), 0644); err != nil {
					return fmt.Errorf("creating example flow: %w", err)
				}
				fmt.Println("  Created flows/example.yaml")
			} else {
				fmt.Println("  flows/example.yaml already exists, skipping")
			}

			fmt.Println("\nNext steps:")
			fmt.Println("  1. Edit flowtest.yaml with your connection strings")
			fmt.Println("  2. flowtest run flows/example.yaml")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "Skip prompts and use defaults (HTTP only)")
	return cmd
}

// isTTY returns true when stdin is an interactive terminal.
func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// promptServices displays a numbered menu and returns the selected service keys.
func promptServices() ([]string, error) {
	fmt.Println("\nFlowTest — select the services you want to test:")
	fmt.Println()
	fmt.Printf("  %-4s %-14s %s\n", "No.", "Service", "Description")
	fmt.Printf("  %-4s %-14s %s\n", "---", "-------", "-----------")
	for i, s := range allServices {
		marker := "  "
		if s.key == "http" {
			marker = "* " // HTTP always included
		}
		fmt.Printf("%s%-4s %-14s %s\n", marker, strconv.Itoa(i+1), s.label, s.desc)
	}
	fmt.Println()
	fmt.Println("  * HTTP API is always included.")
	fmt.Println()
	fmt.Print("Enter numbers to add (e.g. 1,2,5) or press Enter for HTTP only: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	selected := []string{"http"}
	seen := map[string]bool{"http": true}

	if input != "" {
		for _, part := range strings.Split(input, ",") {
			part = strings.TrimSpace(part)
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 || n > len(allServices) {
				fmt.Fprintf(os.Stderr, "  Warning: ignoring invalid entry %q\n", part)
				continue
			}
			key := allServices[n-1].key
			if !seen[key] {
				selected = append(selected, key)
				seen[key] = true
			}
		}
	}

	fmt.Println()
	fmt.Print("  Generating config for:")
	for _, k := range selected {
		for _, s := range allServices {
			if s.key == k {
				fmt.Printf(" %s", s.label)
			}
		}
	}
	fmt.Println()

	return selected, nil
}

// has returns true if key is in the selected slice.
func has(selected []string, key string) bool {
	for _, k := range selected {
		if k == key {
			return true
		}
	}
	return false
}

// buildGlobalConfig generates a flowtest.yaml containing only the selected drivers.
func buildGlobalConfig(selected []string) string {
	var b strings.Builder
	b.WriteString("# FlowTest global configuration\n")
	b.WriteString("# Edit connection strings to match your environment.\n\n")
	b.WriteString("env:\n")

	if has(selected, "http") {
		b.WriteString("  api_base: http://localhost:8000\n")
	}

	hasDbs := has(selected, "postgres") || has(selected, "mysql") || has(selected, "mongodb")
	if hasDbs {
		b.WriteString("\n  databases:\n")
		if has(selected, "postgres") {
			b.WriteString("    db: postgres://user:pass@localhost:5432/myapp?sslmode=disable\n")
		}
		if has(selected, "mysql") {
			b.WriteString("    mysql_db: mysql://user:pass@localhost:3306/myapp\n")
		}
		if has(selected, "mongodb") {
			b.WriteString("    mongo_db: mongodb://localhost:27017/myapp\n")
		}
	}

	if has(selected, "kafka") {
		b.WriteString("\n  kafka_brokers: localhost:9092\n")
	}
	if has(selected, "redis") {
		b.WriteString("\n  redis: redis://localhost:6379\n")
	}

	b.WriteString(`
# profiles:
#   staging:
#     api_base: https://staging.example.com
`)
	return b.String()
}

// buildExampleFlow generates a flows/example.yaml for the selected drivers.
func buildExampleFlow(selected []string) string {
	var b strings.Builder
	b.WriteString("name: Example Flow\n")
	b.WriteString("description: Verify your services are reachable and responding correctly\n")
	b.WriteString("timeout: 30s\n\n")
	b.WriteString("steps:\n")

	if has(selected, "http") {
		b.WriteString(`
  - name: Health Check
    api:
      method: GET
      url: /health
    assert:
      - expr: "response.status == 200"
        msg: "API should return 200"
`)
	}

	if has(selected, "postgres") {
		b.WriteString(`
  - name: Postgres reachable
    db:
      query: "SELECT 1 AS ok"
    assert:
      - expr: "row_count == 1"
        msg: "Postgres should respond"
`)
	}

	if has(selected, "mysql") {
		b.WriteString(`
  - name: MySQL reachable
    mysql_db:
      query: "SELECT 1 AS ok"
    assert:
      - expr: "row_count == 1"
        msg: "MySQL should respond"
`)
	}

	if has(selected, "mongodb") {
		b.WriteString(`
  - name: MongoDB insert and verify
    mongo_db:
      collection: flowtest_ping
      operation: insertOne
      document:
        source: flowtest
        status: ok
    assert:
      - expr: "row_count == 1"
        msg: "MongoDB insert should succeed"
    save:
      ping_id: rows[0]._id

  - name: MongoDB cleanup
    mongo_db:
      collection: flowtest_ping
      operation: deleteOne
      filter:
        source: flowtest
`)
	}

	if has(selected, "redis") {
		b.WriteString(`
  - name: Redis set and get
    redis:
      action: set
      key: "flowtest:ping"
      value: "pong"
      ttl: 1m

  - name: Redis verify
    redis:
      action: get
      key: "flowtest:ping"
    assert:
      - expr: "exists == true"
        msg: "Redis key should exist"
      - expr: "value == 'pong'"
        msg: "Redis value should match"
`)
	}

	if has(selected, "kafka") {
		b.WriteString(`
  - name: Kafka produce
    kafka:
      action: produce
      topic: flowtest.ping
      key: ping
      message:
        source: flowtest
        status: ok

  - name: Kafka consume
    kafka:
      topic: flowtest.ping
      timeout: 10s
      match:
        source: flowtest
    assert:
      - expr: "message.payload.status == 'ok'"
        msg: "Kafka message should be consumable"
`)
	}

	return b.String()
}

func parseVarFlags(vars []string) map[string]interface{} {
	if len(vars) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(vars))
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			var parsed interface{}
			if err := json.Unmarshal([]byte(parts[1]), &parsed); err == nil {
				result[parts[0]] = parsed
			} else {
				result[parts[0]] = parts[1]
			}
		}
	}
	return result
}

