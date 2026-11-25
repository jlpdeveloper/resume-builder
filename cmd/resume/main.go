package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jlpdeveloper/resume-builder/internal/validate"
)

const version = "0.1.0"

type command struct {
	run   func(args []string, stdout, stderr io.Writer, jsonOutput bool) int
	usage string
}

func commands() map[string]command {
	return map[string]command{
		"validate":     {run: validate.Run, usage: "validate PATH"},
		"inspect":      {run: runStub, usage: "inspect PATH"},
		"diff":         {run: runStub, usage: "diff LEFT RIGHT"},
		"build":        {run: runStub, usage: "build PATH"},
		"dependencies": {run: runStub, usage: "dependencies PATH"},
		"version":      {run: runVersion, usage: "version"},
	}
}

func runStub(_ []string, _, stderr io.Writer, jsonOutput bool) int {
	if jsonOutput {
		_ = json.NewEncoder(stderr).Encode(map[string]string{"error": "not yet implemented"})
	} else {
		fmt.Fprintln(stderr, "not yet implemented")
	}
	return 1
}

func runVersion(_ []string, stdout, _ io.Writer, _ bool) int {
	fmt.Fprintf(stdout, "resume %s\n", version)
	return 0
}

func main() {
	os.Exit(execute(os.Stdout, os.Stderr))
}

func execute(stdout, stderr io.Writer) int {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsageTo(stdout)
		return 1
	}

	jsonFlag := false
	cmdArgs := []string{}
	for _, a := range args {
		if a == "--json" {
			jsonFlag = true
		} else {
			cmdArgs = append(cmdArgs, a)
		}
	}

	if len(cmdArgs) == 0 {
		printUsageTo(stdout)
		return 1
	}

	cmdName := cmdArgs[0]
	cmds := commands()

	if cmdName == "help" || cmdName == "--help" || cmdName == "-h" {
		printUsageTo(stdout)
		return 0
	}

	cmd, ok := cmds[cmdName]
	if !ok {
		if jsonFlag {
			enc := json.NewEncoder(stderr)
			enc.Encode(map[string]string{"error": "unknown command: " + cmdName})
		} else {
			fmt.Fprintf(stderr, "unknown command: %s\n", cmdName)
		}
		return 1
	}

	return cmd.run(cmdArgs[1:], stdout, stderr, jsonFlag)
}

func printUsageTo(w io.Writer) {
	fmt.Fprintln(w, "Usage: resume <command> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, name := range []string{"validate", "inspect", "diff", "build", "dependencies", "version"} {
		fmt.Fprintf(w, "  %-14s %s\n", name, commands()[name].usage)
	}
}
