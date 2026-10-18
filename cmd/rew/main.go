package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: rew <command>")
		return 2
	}

	switch args[0] {
	case "--help", "-h", "help":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: rew help")
			return 2
		}
		printGeneralHelp()
		return 0

	case "--version":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: rew --version")
			return 2
		}
		fmt.Println("rew " + rewVersion())
		return 0
	case "splash":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: rew splash")
			return 2
		}
		PrintSplash(rewVersion())
		return 0

	case "capture-demo":
		return runCaptureDemo(args[1:])

	case "version":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: rew version")
			return 2
		}
		fmt.Println("rew " + rewVersion())
		return 0

	case "capture":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			printCaptureHelp()
			return 0
		}

		if len(args) >= 2 && args[1] == "--v3" {
			var includes []string
			includeGoEmbed := false
			includeGoSources := false
			i := 2

			for i < len(args) {
				switch args[i] {
				case "--include":
					if i+1 >= len(args) ||
						args[i+1] == "" ||
						strings.HasPrefix(args[i+1], "--") {
						fmt.Fprintln(os.Stderr,
							"rew: --include requires a file path")
						return 2
					}
					includes = append(includes, args[i+1])
					i += 2

				case "--include-go-sources":
					if includeGoSources {
						fmt.Fprintln(os.Stderr,
							"rew: duplicate --include-go-sources")
						return 2
					}
					includeGoSources = true
					i++

				case "--include-go-embed":
					if includeGoEmbed {
						fmt.Fprintln(os.Stderr,
							"rew: duplicate --include-go-embed")
						return 2
					}
					includeGoEmbed = true
					i++

				default:
					goto captureV3OptionsDone
				}
			}
		captureV3OptionsDone:

			if (!includeGoSources && len(includes) == 0) ||
				i >= len(args) ||
				args[i] != "--" ||
				i+1 >= len(args) {
				fmt.Fprintln(os.Stderr,
					"Usage: rew capture --v3 "+
						"[--include <file> ... | --include-go-sources] "+
						"[--include-go-embed] -- <command> [args...]")
				return 2
			}

			return captureCommandV3WithDiscovery(
				args[i+1:],
				".",
				includes,
				"failure.rew",
				includeGoEmbed,
				includeGoSources,
			)
		}

		if len(args) >= 3 && args[1] == "--" {
			return captureCommand(args[2:], "failure.bfc", false)
		}

		var includes []string
		i := 1

		for i < len(args) && args[i] == "--include" {
			if i+1 >= len(args) || args[i+1] == "" || args[i+1] == "--" {
				fmt.Fprintln(os.Stderr, "rew: --include requires a file path")
				return 2
			}

			includes = append(includes, args[i+1])
			i += 2
		}

		if len(includes) == 0 || i >= len(args) || args[i] != "--" || i+1 >= len(args) {
			fmt.Fprintln(
				os.Stderr,
				"Usage: rew capture [--include <file> ...] -- <command> [args...]",
			)
			return 2
		}

		return captureCommandV2(
			args[i+1:],
			".",
			includes,
			"failure.bfc",
		)

	case "audit":
		if len(args) == 2 &&
			(args[1] == "--help" || args[1] == "-h") {
			printAuditHelp()
			return 0
		}
		return runAudit(args[1:])

	case "cleanup":
		return runCleanup(args[1:])

	case "replay":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			printReplayHelp()
			return 0
		}
		return runReplay(args[1:])

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		return 2
	}
}
