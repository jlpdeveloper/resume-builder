package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const version = "0.1.0"

type command struct {
	run    func(args []string) error
	usage  string
	output string
}

func commands() map[string]command {
	return map[string]command{
		"validate":     {run: stub, usage: "validate PATH"},
		"inspect":      {run: stub, usage: "inspect PATH"},
		"diff":         {run: stub, usage: "diff LEFT RIGHT"},
		"build":        {run: stub, usage: "build PATH"},
		"schema":       {run: stub, usage: "schema KIND"},
		"dependencies": {run: stub, usage: "dependencies PATH"},
		"version":      {run: runVersion, usage: "version", output: "resume " + version + "\n"},
	}
}

func stub(_ []string) error {
	return fmt.Errorf("not yet implemented")
}

func runVersion(_ []string) error {
	return nil
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

	if err := cmd.run(cmdArgs[1:]); err != nil {
		if jsonFlag {
			enc := json.NewEncoder(stderr)
			enc.Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}

	if cmd.output != "" {
		fmt.Fprint(stdout, cmd.output)
	}

	return 0
}

func printUsageTo(w io.Writer) {
	fmt.Fprintln(w, "Usage: resume <command> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, name := range []string{"validate", "inspect", "diff", "build", "schema", "dependencies", "version"} {
		fmt.Fprintf(w, "  %-14s %s\n", name, commands()[name].usage)
	}
}
