package main

import (
	"flag"
	"fmt"
	"os"

	"fluid/probes/core/internal/scaffold"
)

func main() {
	scaffoldCmd := flag.NewFlagSet("scaffold", flag.ExitOnError)
	probeName := scaffoldCmd.String("name", "", "Probe name (required)")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "scaffold":
		scaffoldCmd.Parse(os.Args[2:])
		if *probeName == "" {
			fmt.Fprintf(os.Stderr, "Error: probe name is required\n")
			fmt.Fprintf(os.Stderr, "Usage: core scaffold -name <probe-name>\n")
			os.Exit(1)
		}

		if err := scaffold.Generate(*probeName); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating scaffold: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Probe scaffold '%s' generated successfully!\n", *probeName)
		fmt.Printf("Next steps:\n")
		fmt.Printf("  1. cd ../%s\n", *probeName)
		fmt.Printf("  2. Update config/probe.yml with your service configuration\n")
		fmt.Printf("  3. Implement your entities in internal/probe/entities/\n")
		fmt.Printf("  4. Implement your service client in internal/<service>/\n")
		fmt.Printf("  5. Run 'make dev' to start developing\n")

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: core <command> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  scaffold    Generate a new probe scaffold\n")
	fmt.Fprintf(os.Stderr, "              Usage: core scaffold -name <probe-name>\n")
}
