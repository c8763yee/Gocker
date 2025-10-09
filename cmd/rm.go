// gocker/cmd/rm.go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gocker/internal"
	"gocker/internal/image"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rmCommand = &cobra.Command{
	Use:   "rm [CONTAINER_ID]",
	Short: "Remove a container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		identifier := args[0]

		if identifier == "all" {
			removeAllContainers()
			return
		}

		container, err := resolveContainer(identifier)
		if err != nil {
			logrus.Fatalf("%v", err)
		}

		if err := internal.RemoveContainer(container.ID); err != nil {
			logrus.Fatalf("Failed to remove container %s: %v", identifier, err)
		}
		fmt.Println(container.ID)
	},
}

func init() {
	rootCmd.AddCommand(rmCommand)
}

func resolveContainer(identifier string) (*types.ContainerInfo, error) {
	containers, err := image.ListContainers()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var matches []*types.ContainerInfo
	for _, c := range containers {
		if c.ID == identifier || c.Name == identifier || strings.HasPrefix(c.ID, identifier) {
			matches = append(matches, c)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("cannot find container: %s", identifier)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("found multiple containers matching %s, please specify a more precise ID", identifier)
	}
}

func removeAllContainers() {
	containers, err := image.ListContainers()
	if err != nil {
		logrus.Fatalf("Failed to list containers: %v", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers to remove.")
		return
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Are you sure you want to remove all %d containers? (y/n): ", len(containers))
	input, err := reader.ReadString('\n')
	if err != nil {
		logrus.Fatalf("Failed to read input: %v", err)
	}

	choice := strings.TrimSpace(strings.ToLower(input))
	if choice != "y" && choice != "yes" {
		fmt.Println("Cancelled removal of all containers.")
		return
	}

	removed := 0
	for _, c := range containers {
		if err := internal.RemoveContainer(c.ID); err != nil {
			logrus.Errorf("Failed to remove container %s (%s): %v", c.Name, c.ID, err)
			continue
		}
		fmt.Printf("Removed container %s (%s)\n", c.Name, c.ID)
		removed++
	}

	if removed == 0 {
		fmt.Println("No containers were removed.")
	} else {
		fmt.Printf("Successfully removed %d containers.\n", removed)
	}
}
