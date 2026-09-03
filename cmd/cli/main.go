package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var (
	version   = "v2.0.0"
	endpoint  string
	apiKey    string
	env       string
	projectID string
	jsonOut   bool
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	// Read environment variables
	defaultEndpoint := os.Getenv("FLAGURA_ENDPOINT")
	if defaultEndpoint == "" {
		defaultEndpoint = "http://localhost:3000"
	}
	defaultApiKey := os.Getenv("FLAGURA_API_KEY")
	defaultProject := os.Getenv("FLAGURA_PROJECT_ID")

	cmd := os.Args[1]

	// Global Flags
	fs := flag.NewFlagSet("flagura", flag.ExitOnError)
	fs.StringVar(&endpoint, "endpoint", defaultEndpoint, "Flagura control plane URL")
	fs.StringVar(&apiKey, "api-key", defaultApiKey, "API key for authentication")
	fs.StringVar(&env, "env", "production", "Target environment (production, staging, development)")
	fs.StringVar(&projectID, "project", defaultProject, "Project ID scope (env: FLAGURA_PROJECT_ID)")
	fs.BoolVar(&jsonOut, "json", false, "Output results as formatted JSON")

	switch cmd {
	case "version", "-v", "--version":
		fmt.Printf("Flagura CLI %s\n", version)
	case "list", "ls":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		runList()
	case "get":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura get <flag-key>\n")
			os.Exit(1)
		}
		runGet(fs.Arg(0))
	case "toggle":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura toggle <flag-key> [--env=production]\n")
			os.Exit(1)
		}
		runToggle(fs.Arg(0))
	case "rollout":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura rollout <flag-key> <percentage> [--env=production]\n")
			os.Exit(1)
		}
		pct, err := strconv.ParseFloat(strings.TrimSuffix(fs.Arg(1), "%"), 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid rollout percentage: %v\n", fs.Arg(1))
			os.Exit(1)
		}
		runRollout(fs.Arg(0), pct)
	case "evaluate", "eval":
		userFlag := fs.String("user", "usr_cli_01", "User ID for evaluation context")
		emailFlag := fs.String("email", "dev@flagura.dev", "Email for evaluation context")
		traceFlag := fs.Bool("trace", false, "Include step-by-step visual execution trace")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura evaluate <flag-key> [--user=<id>] [--email=<email>] [--trace]\n")
			os.Exit(1)
		}
		runEvaluate(fs.Arg(0), *userFlag, *emailFlag, *traceFlag)
	case "promote":
		fromFlag := fs.String("from", "staging", "Source environment")
		toFlag := fs.String("to", "production", "Destination environment")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura promote <flag-key> [--from=staging] [--to=production]\n")
			os.Exit(1)
		}
		runPromote(fs.Arg(0), *fromFlag, *toFlag)
	case "experiment", "exp":
		metricFlag := fs.String("metric", "conversion", "Metric name to analyze")
		controlFlag := fs.String("control", "control", "Control/baseline variant key")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura experiment <flag-key> [--metric=conversion] [--control=control]\n")
			os.Exit(1)
		}
		runExperiment(fs.Arg(0), *metricFlag, *controlFlag)
	case "canary":
		stagesFlag := fs.String("stages", "5%:5m,25%:30m,50%:1h,100%:0s", "Progressive stages (<pct>:<duration>,...)")
		rollbackFlag := fs.Bool("rollback", false, "Trigger immediate health guardrail rollback to 0%")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Usage: flagura canary <flag-key> [--stages=<stages>] [--rollback]\n")
			os.Exit(1)
		}
		runCanary(fs.Arg(0), *stagesFlag, *rollbackFlag)
	case "change-request", "cr":
		statusFlag := fs.String("status", "", "Filter status: PENDING, APPROVED, REJECTED, APPLIED")
		commentsFlag := fs.String("comments", "", "Review comments")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		subCmd := "list"
		if fs.NArg() > 0 {
			subCmd = fs.Arg(0)
		}
		runChangeRequest(subCmd, fs.Args(), *statusFlag, *commentsFlag)
	case "api-key", "apikey", "key":
		nameFlag := fs.String("name", "CLI Service Key", "Descriptive name for the API Key")
		roleFlag := fs.String("role", "developer", "Assigned role: developer or admin")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		subCmd := "list"
		if fs.NArg() > 0 {
			subCmd = fs.Arg(0)
		}
		runAPIKey(subCmd, fs.Args(), *nameFlag, *roleFlag)
	case "audit", "scan", "clean-up", "cleanup":
		dirFlag := fs.String("dir", ".", "Root directory of codebase to scan")
		failOnStale := fs.Bool("fail-on-stale", false, "Exit with error code 1 if stale flags are detected")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		runAudit(*dirFlag, *failOnStale)
	case "health":
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(1)
		}
		runHealth()
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'flagura help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`⚡ Flagura Developer CLI (%s)
High-performance feature flag management and terminal evaluation engine.

Usage:
  flagura <command> [arguments] [flags]

Commands:
  list, ls                  List all feature flags across environments
  get <key>                 View detailed configuration for a specific flag
  toggle <key>              Instantly toggle the master kill-switch for a flag
  rollout <key> <pct>       Adjust gradual rollout percentage (0-100%%)
  evaluate <key>            Execute real-time flag evaluation in terminal
  promote <key>             Promote configuration from Staging to Production
  experiment <key>          Calculate A/B statistical significance and lift
  canary <key>              Manage automated progressive canary ramp & guardrails
  change-request [list|approve|reject|apply]
                            Enforce 4-Eyes principle change approval governance
  api-key [list|create|revoke]
                            Generate, list, and revoke programmatic API service tokens
  audit, scan, clean-up     Scan codebase files for technical debt & stale flag checks
  health                    Check connection to the Flagura control plane
  version                   Print CLI version
  help                      Show this help message

Flags:
  --endpoint <url>          Flagura control plane URL (default: $FLAGURA_ENDPOINT or http://localhost:3000)
  --api-key <key>           API key for authentication (default: $FLAGURA_API_KEY)
  --env <name>              Target environment: production, staging, development (default: production)
  --name <name>             Name for newly generated API Key (default: CLI Service Key)
  --role <role>             Role for API Key: developer or admin (default: developer)
  --dir <path>              Codebase directory to scan during audit (default: .)
  --fail-on-stale           Exit with code 1 if stale/dead flags are found (useful in CI/CD)
  --json                    Output results as raw formatted JSON

Examples:
  flagura list
  flagura toggle ai-smart-search --env=production
  flagura rollout ai-smart-search 50%% --env=production
  flagura evaluate ai-smart-search --user=usr_alex_42 --trace
  flagura promote ai-smart-search --from=staging --to=production
  flagura api-key create --name="Production Kubernetes SDK" --role=admin
  flagura api-key list
  flagura audit --dir=. --fail-on-stale
`, version)
}
