package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

// Server defines the structure matching your JSON file
type Server struct {
	IP        string `json:"ssh_host"`
	Port      int    `json:"ssh_port"`
	SSHKey    string `json:"ssh_key"`
	User      string `json:"ssh_user"`
	SSHKEYDIR string `json:"ssh_key_dir"`
}

const (
	GREEN  = "\033[92m"
	YELLOW = "\033[93m"
	GREY   = "\033[90m"
	RESET  = "\033[0m"
)

func printUsage() {
	usageText := fmt.Sprintf(`Usage: 
    ./ssh_menu 		%s // Launches full interactive menu%s
    ./ssh_menu i %s<keyword>%s	 // EXAMPLE: %s./ssh_menu i a%s for interactive menu with servers starting with A%s
    ./ssh_menu %s<server-name>%s // Connect directly%s
    ./ssh_menu list
    ./ssh_menu list %s<keyword>%s// EXAMPLE: %s./ssh_menu list a%s for text list of servers starting with A%s
`, GREY, RESET, GREEN, GREY, YELLOW, GREY, RESET, GREEN, GREY, RESET, GREEN, GREY, YELLOW, GREY, RESET)
	fmt.Println(usageText)
}

func get_script_dir() string {
	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}

	// NEW: Fallback for when you use 'go run'
	if strings.Contains(exePath, "go-build") {
		cwd, err := os.Getwd()
		if err == nil {
			return cwd
		}
	}

	// Resolve any symlinks to find the true location of the binary
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		realPath = exePath
	}

	return filepath.Dir(realPath)
}

func loadServers() map[string]Server {
	jsonPath := filepath.Join(get_script_dir(), "server.json")

	file, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Printf("✗ Error: Could not find 'server.json' at %s\n", jsonPath)
		os.Exit(1)
	}

	// Load it as a flat map, just like your original Python script
	var servers map[string]Server
	if err := json.Unmarshal(file, &servers); err != nil {
		fmt.Println("✗ Error: Could not parse 'server.json'.")
		os.Exit(1)
	}

	// 1. Check if the special "_constants" block exists
	var defaultUser string
	var defaultPort int
	var defaultSSHkeydir string

	if constants, exists := servers["_constants"]; exists {
		defaultUser = constants.User
		defaultPort = constants.Port
		defaultSSHkeydir = constants.SSHKEYDIR

		// Delete it from the map so it doesn't appear in your terminal menu
		delete(servers, "_constants")
	}

	// 2. Loop through the real servers and apply defaults
	for name, server := range servers {
		if server.User == "" {
			if defaultUser != "" {
				server.User = defaultUser
			} else {
				server.User = "ec2-user" // Ultimate fallback if no constants are provided
			}
		}

		if server.Port == 0 {
			if defaultPort != 0 {
				server.Port = defaultPort
			} else {
				server.Port = 22 // Standard SSH port fallback
			}
		}

		if server.SSHKEYDIR == "" {
			if defaultSSHkeydir != "" {
				server.SSHKEYDIR = defaultSSHkeydir
			} else {
				server.SSHKEYDIR = get_script_dir()
			}
		}

		// Save the updated server back into the map
		servers[name] = server
	}

	return servers
}

func getFilteredServers(servers map[string]Server, keyword string) []string {
	keyword = strings.ToUpper(keyword)
	var serverNames []string
	for name := range servers {
		if strings.HasPrefix(strings.ToUpper(name), keyword) {
			serverNames = append(serverNames, name)
		}
	}
	sort.Strings(serverNames)
	return serverNames
}

func runInteractiveMenu(servers map[string]Server, serverNames []string) {
	if len(serverNames) == 0 {
		fmt.Println("No matching servers found.")
		os.Exit(1)
	}

	var selectedServer string
	prompt := &survey.Select{
		Message: "Use arrow keys to select a server (or press Ctrl+C to exit):",
		Options: serverNames,
	}

	err := survey.AskOne(prompt, &selectedServer)
	if err != nil {
		printUsage()
		os.Exit(0)
	}

	executeSSH(servers, selectedServer)
}

func executeSSH(servers map[string]Server, serverName string) {
	server, exists := servers[strings.ToUpper(serverName)]
	if !exists {
		fmt.Printf("%sTip: Run %s./ssh_menu list%s to see available servers.%s\n", GREY, GREEN, GREY, RESET)
		os.Exit(1)
	}

	if server.User == "" {
		server.User = "ec2-user"
	}

	fmt.Printf("Connecting to %s\n", strings.ToUpper(serverName))

	target := fmt.Sprintf("%s@%s", server.User, server.IP)
	portStr := fmt.Sprintf("%d", server.Port)

	// Assuming ssh-key folder structure from your script
	keyPath := fmt.Sprintf("%s/%s", server.SSHKEYDIR, server.SSHKey)
	fmt.Printf("ssh -i %s -p %s %s", keyPath, portStr, target)
	cmd := exec.Command("ssh", "-i", keyPath, "-p", portStr, target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("SSH connection closed or failed: %v\n", err)
	}
	os.Exit(0)
}

func main() {
	args := os.Args

	// --- 1. Interactive Menu (No arguments) ---
	if len(args) < 2 {
		servers := loadServers()
		serverNames := getFilteredServers(servers, "")
		runInteractiveMenu(servers, serverNames)
		return
	}

	// --- 2. Handle Arguments ---
	argument := strings.ToLower(args[1])
	servers := loadServers()

	// Optional catch-all for help
	if argument == "help" || argument == "--help" || argument == "-h" {
		printUsage()
		os.Exit(0)
	}

	// --- 3. Filtered Interactive Menu ---
	if argument == "i" || argument == "interactive" {
		keyword := ""
		if len(args) > 2 {
			keyword = args[2]
		}
		serverNames := getFilteredServers(servers, keyword)
		runInteractiveMenu(servers, serverNames)
		return
	}

	// --- 4. Text List Feature ---
	if argument == "list" {
		keyword := ""
		if len(args) > 2 {
			keyword = args[2]
		}
		serverNames := getFilteredServers(servers, keyword)

		fmt.Println("Available servers:")
		for _, name := range serverNames {
			fmt.Printf("  - %s\n", name)
		}
		os.Exit(0)
	}

	// --- 5. Direct Connection ---
	// If it doesn't match any commands above, assume it's a server name
	executeSSH(servers, argument)
}
