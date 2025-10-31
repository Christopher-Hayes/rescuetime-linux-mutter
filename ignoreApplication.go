// +build ignore_app

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	monitorDuration = 10 * time.Second
	pollInterval    = 500 * time.Millisecond
	ignoreFilePath  = ".rescuetime-ignore"
)

// SeenApplication tracks when we saw an application
type SeenApplication struct {
	WmClass     string
	LastSeen    time.Time
	WindowTitle string
}

// getWindowFromDBus queries the GNOME Shell FocusedWindow extension
func getWindowFromDBus() (*MutterWindow, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %v", err)
	}
	defer conn.Close()

	obj := conn.Object(dbusDestination, dbusObjectPath)
	call := obj.Call(dbusMethod, 0)

	if call.Err != nil {
		return nil, fmt.Errorf("failed to call FocusedWindow.Get: %v", call.Err)
	}

	var jsonStr string
	err = call.Store(&jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse D-Bus response: %v", err)
	}

	var window MutterWindow
	err = json.Unmarshal([]byte(jsonStr), &window)
	if err != nil {
		return nil, fmt.Errorf("failed to parse window JSON: %v", err)
	}

	return &window, nil
}

// loadCurrentIgnoreList reads the current ignore list from file
func loadCurrentIgnoreList() map[string]bool {
	ignoredApps := make(map[string]bool)

	file, err := os.Open(ignoreFilePath)
	if err != nil {
		// File doesn't exist yet, that's ok
		return ignoredApps
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ignoredApps[line] = true
	}

	return ignoredApps
}

// saveIgnoreList saves the ignore list to file
func saveIgnoreList(ignoredApps map[string]bool) error {
	file, err := os.Create(ignoreFilePath)
	if err != nil {
		return fmt.Errorf("failed to create ignore file: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// Write header
	fmt.Fprintln(writer, "# RescueTime Ignored Applications")
	fmt.Fprintln(writer, "# One WmClass per line")
	fmt.Fprintln(writer, "# Lines starting with # are comments")
	fmt.Fprintln(writer, "")

	// Write ignored apps
	for appClass := range ignoredApps {
		fmt.Fprintln(writer, appClass)
	}

	return writer.Flush()
}

func main() {
	log.SetFlags(0) // No timestamps for this interactive tool

	fmt.Println("=== RescueTime Application Ignore Tool ===")
	fmt.Println()
	fmt.Println("This tool will monitor your active windows for the next 10 seconds.")
	fmt.Println("Switch between applications you want to review.")
	fmt.Println()
	fmt.Print("Press Enter to start monitoring...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	// Check D-Bus connection
	_, err := getWindowFromDBus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to GNOME Shell FocusedWindow extension.\n")
		fmt.Fprintf(os.Stderr, "Make sure the extension is installed and enabled.\n")
		fmt.Fprintf(os.Stderr, "Details: %v\n", err)
		os.Exit(1)
	}

	// Monitor windows
	fmt.Printf("\nMonitoring for %v...\n", monitorDuration)
	seenApps := make(map[string]*SeenApplication)

	startTime := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	for time.Since(startTime) < monitorDuration {
		select {
		case <-ticker.C:
			window, err := getWindowFromDBus()
			if err != nil {
				continue
			}

			if window.WmClass != "" {
				if _, exists := seenApps[window.WmClass]; !exists {
					fmt.Printf("  Found: %s\n", window.WmClass)
				}
				seenApps[window.WmClass] = &SeenApplication{
					WmClass:     window.WmClass,
					LastSeen:    time.Now(),
					WindowTitle: window.Title,
				}
			}

		case <-progressTicker.C:
			elapsed := time.Since(startTime)
			remaining := monitorDuration - elapsed
			fmt.Printf("  %v remaining... (%d apps found)\n", remaining.Round(time.Second), len(seenApps))
		}
	}

	fmt.Printf("\nFound %d unique applications.\n\n", len(seenApps))

	if len(seenApps) == 0 {
		fmt.Println("No applications detected. Make sure you switched between some windows.")
		os.Exit(0)
	}

	// Load current ignore list
	currentlyIgnored := loadCurrentIgnoreList()

	// Display applications
	fmt.Println("Applications detected:")
	fmt.Println()

	appList := make([]*SeenApplication, 0, len(seenApps))
	for _, app := range seenApps {
		appList = append(appList, app)
	}

	// Sort by last seen (most recent first)
	for i := 0; i < len(appList)-1; i++ {
		for j := i + 1; j < len(appList); j++ {
			if appList[j].LastSeen.After(appList[i].LastSeen) {
				appList[i], appList[j] = appList[j], appList[i]
			}
		}
	}

	// Display numbered list
	for i, app := range appList {
		status := ""
		if currentlyIgnored[app.WmClass] {
			status = " [ALREADY IGNORED]"
		}
		fmt.Printf("  %d) %s%s\n", i+1, app.WmClass, status)
		if app.WindowTitle != "" {
			fmt.Printf("     Last window: %s\n", app.WindowTitle)
		}
	}

	fmt.Println()
	fmt.Println("Enter the number of the application to ignore (or 0 to cancel):")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 0 || choice > len(appList) {
		fmt.Println("Invalid choice. Exiting.")
		os.Exit(0)
	}

	if choice == 0 {
		fmt.Println("Cancelled.")
		os.Exit(0)
	}

	// Add to ignore list
	selectedApp := appList[choice-1]

	if currentlyIgnored[selectedApp.WmClass] {
		fmt.Printf("\n'%s' is already in the ignore list.\n", selectedApp.WmClass)
		os.Exit(0)
	}

	currentlyIgnored[selectedApp.WmClass] = true

	err = saveIgnoreList(currentlyIgnored)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving ignore list: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Added '%s' to ignore list (%s)\n", selectedApp.WmClass, ignoreFilePath)
	fmt.Println()
	fmt.Println("This application will now be excluded from RescueTime tracking.")
	fmt.Println("Restart active-window if it's currently running to apply changes.")
}
