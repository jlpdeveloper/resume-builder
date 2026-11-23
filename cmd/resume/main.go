package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const version = "0.1.0"

type command struct {
	run    func(args []string) error
	usage  string
}

func commands() map[string]command {
	return map[string]command{
		"validate":     {run: stub, usage: "validate PATH"},
		"inspect":      {run: stub, usage: "inspect PATH"},
		"diff":         {run: stub, usage: "diff LEFT RIGHT"},
		"build":        {run: stub, usage: "build PATH"},
		"schema":       {run: stub, usage: "schema KIND"},
		"dependencies": {run: stub, usage: "dependencies PATH"},
		"version":      {run: runVersion, usage: "version"},
	}
}

func stub(_ []string) error {
	return fmt.Errorf("not yet implemented")
}

func runVersion(_ []string) error {
	fmt.Printf("resume %s\n", version)
	return nil
}

func printUsage() {
	fmt.Println("Usage: resume <command> [args...]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, name := range []string{"validate", "inspect", "diff", "build", "schema", "dependencies", "version"} {
		fmt.Printf("  %-14s %s\n", name, commands()[name].usage)
	}
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
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
		printUsage()
		return 1
	}

	cmdName := cmdArgs[0]
	cmds := commands()

	if cmdName == "help" || cmdName == "--help" || cmdName == "-h" {
		printUsage()
		return 0
	}

	cmd, ok := cmds[cmdName]
	if !ok {
		if jsonFlag {
			enc := json.NewEncoder(os.Stderr)
			enc.Encode(map[string]string{"error": "unknown command: " + cmdName})
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmdName)
		}
		return 1
	}

	if err := cmd.run(cmdArgs[1:]); err != nil {
		if jsonFlag {
			enc := json.NewEncoder(os.Stderr)
			enc.Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return 1
	}

	return 0
}
