package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Hyeong-soo/randomChat/internal/config"
	"github.com/Hyeong-soo/randomChat/internal/tui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "logout":
			if err := cfg.ClearSession(); err != nil {
				if os.IsNotExist(err) {
					fmt.Println("Not logged in.")
				} else {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Println("Logged out successfully.")
			}
			return
		case "help", "--help", "-h":
			fmt.Println("Usage: randomchat [command]")
			fmt.Println()
			fmt.Println("Commands:")
			fmt.Println("  (none)    Start RandomChat")
			fmt.Println("  logout    Log out and clear session")
			fmt.Println("  help      Show this help")
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			fmt.Fprintln(os.Stderr, "Run 'randomchat help' for usage.")
			os.Exit(1)
		}
	}

	// Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	app := tui.NewApp(cfg)
	p := tea.NewProgram(&app, tea.WithAltScreen())
	app.SetProgram(p)

	go func() {
		<-sigCh
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
