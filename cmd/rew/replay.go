package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/rewtools/rew/internal/replay"
)

func replayContext(
	parent context.Context,
	useV2 bool,
) (context.Context, context.CancelFunc) {
	if useV2 {
		return context.WithCancel(parent)
	}

	return context.WithTimeout(parent, 30*time.Second)
}

func runReplay(args []string) int {
	const usage = "Usage:\n" +
		"  rew replay [--allow-command] <capsule.bfc>\n" +
		"  rew replay --v2 --allow-command [--diagnose-dependencies] <capsule.bfc>\n" +
		"  rew replay --v3 --allow-command [--memory 256m|512m|1g|2g] [--show-stderr] <capsule.rew>\n" +
		"  rew replay --v4 --allow-command [--memory 256m|512m|1g|2g] [--show-stderr] <capsule.rew>"

	var capsulePath string
	var options replay.Options
	var useV2 bool
	var useV3 bool
	var useV4 bool

	switch {
	case len(args) == 1:
		capsulePath = args[0]

	case len(args) == 2 && args[0] == "--allow-command":
		options.AllowArbitraryCommand = true
		capsulePath = args[1]

	case len(args) == 3 &&
		args[0] == "--v2" &&
		args[1] == "--allow-command":
		useV2 = true
		options.AllowArbitraryCommand = true
		capsulePath = args[2]

	case len(args) == 4 &&
		args[0] == "--v2" &&
		args[1] == "--allow-command" &&
		args[2] == "--diagnose-dependencies":
		useV2 = true
		options.AllowArbitraryCommand = true
		options.DiagnoseDependencies = true
		capsulePath = args[3]

	case len(args) == 3 &&
		args[0] == "--v3" &&
		args[1] == "--allow-command":
		useV3 = true
		options.AllowArbitraryCommand = true
		capsulePath = args[2]

	case len(args) == 5 &&
		args[0] == "--v3" &&
		args[1] == "--allow-command" &&
		args[2] == "--memory":
		memory, err := replay.ParseReplayMemoryV3(args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		useV3 = true
		options.AllowArbitraryCommand = true
		options.MemoryBytes = memory
		capsulePath = args[4]

	case len(args) == 4 &&
		args[0] == "--v3" &&
		args[1] == "--allow-command" &&
		args[2] == "--show-stderr":
		useV3 = true
		options.AllowArbitraryCommand = true
		options.ShowStderr = true
		capsulePath = args[3]

	case len(args) == 6 &&
		args[0] == "--v3" &&
		args[1] == "--allow-command" &&
		args[2] == "--memory" &&
		args[4] == "--show-stderr":
		memory, err := replay.ParseReplayMemoryV3(args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		useV3 = true
		options.AllowArbitraryCommand = true
		options.MemoryBytes = memory
		options.ShowStderr = true
		capsulePath = args[5]

	case len(args) == 3 &&
		args[0] == "--v4" &&
		args[1] == "--allow-command":
		useV4 = true
		options.AllowArbitraryCommand = true
		capsulePath = args[2]

	case len(args) == 5 &&
		args[0] == "--v4" &&
		args[1] == "--allow-command" &&
		args[2] == "--memory":
		memory, err := replay.ParseReplayMemoryV3(args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		useV4 = true
		options.AllowArbitraryCommand = true
		options.MemoryBytes = memory
		capsulePath = args[4]

	case len(args) == 4 &&
		args[0] == "--v4" &&
		args[1] == "--allow-command" &&
		args[2] == "--show-stderr":
		useV4 = true
		options.AllowArbitraryCommand = true
		options.ShowStderr = true
		capsulePath = args[3]

	case len(args) == 6 &&
		args[0] == "--v4" &&
		args[1] == "--allow-command" &&
		args[2] == "--memory" &&
		args[4] == "--show-stderr":
		memory, err := replay.ParseReplayMemoryV3(args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		useV4 = true
		options.AllowArbitraryCommand = true
		options.MemoryBytes = memory
		options.ShowStderr = true
		capsulePath = args[5]

	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}

	if capsulePath == "" || strings.HasPrefix(capsulePath, "--") {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}

	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
	)
	defer stop()

	ctx, cancel := replayContext(signalCtx, useV2 || useV3 || useV4)
	defer cancel()

	var result replay.Result
	var err error

	switch {
	case useV4:
		result, err = runPinnedV4ReplayPath(
			ctx, capsulePath, options,
		)
	case useV3:
		result, err = replay.RunV3WithOptions(ctx, capsulePath, options)
	case useV2:
		result, err = replay.RunV2WithOptions(ctx, capsulePath, options)
	default:
		result, err = replay.RunWithOptions(ctx, capsulePath, options)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "rew: %v\n", err)
		return 125
	}

	fmt.Printf("Original exit code: %d\n", result.OriginalExitCode)
	fmt.Printf("Replay exit code:   %d\n", result.ReplayExitCode)
	fmt.Printf("Outcome:            %s\n", result.Outcome)

	if (useV3 || useV4) && result.Outcome == replay.OutcomeDifferent {
		switch result.Difference {
		case replay.DifferenceExitCode:
			fmt.Println("Difference:         exit codes differ")
		case replay.DifferenceStderr:
			fmt.Println("Difference:         stderr differs")
		}
	}

	if (useV3 || useV4) && options.ShowStderr && result.StderrDiagnostics != nil {
		d := result.StderrDiagnostics
		printReplayStderrDiagnostics(
			os.Stdout,
			d.Original,
			d.OriginalTruncated,
			d.Replayed,
			d.ReplayTruncated,
			d.OriginalDisplayTruncated,
			d.ReplayDisplayTruncated,
		)
	}

	if (useV3 || useV4) && options.MemoryBytes != 0 {
		printReplayMemory(os.Stdout, options.MemoryBytes)
	}

	if result.GoDependencies != nil {
		fmt.Printf("Go dependencies:    %s\n", result.GoDependencies.State)

		if result.GoDependencies.Reason != "" {
			fmt.Printf("Dependency details: %s\n", result.GoDependencies.Reason)
		}
	}

	switch result.Outcome {
	case replay.OutcomeReproduced:
		return 0
	case replay.OutcomePassed:
		return 10
	case replay.OutcomeDifferent:
		return 11
	case replay.OutcomeInconclusive:
		return 12
	default:
		return 125
	}
}

func printReplayMemory(w io.Writer, memoryBytes int64) {
	fmt.Fprintf(w, "Replay memory:      %s\n", replayMemoryLabel(memoryBytes))
}

func replayMemoryLabel(memoryBytes int64) string {
	switch memoryBytes {
	case 256 << 20:
		return "256 MiB"
	case 512 << 20:
		return "512 MiB"
	case 1 << 30:
		return "1 GiB"
	case 2 << 30:
		return "2 GiB"
	default:
		return "unknown"
	}
}
