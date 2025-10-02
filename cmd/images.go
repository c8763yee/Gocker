// cmd/images.go
package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gocker/internal/image"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List all locally stored images",
	Long:  `Lists all images that have been pulled and are stored locally in the gocker storage path.`,
	Run: func(cmd *cobra.Command, args []string) {
		mgr := image.NewManager()
		localImages, err := mgr.ListImages()
		if err != nil {
			logrus.Fatalf("Failed to list images: %v", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
		fmt.Fprint(w, "REPOSITORY\tTAG\n")
		for _, imgName := range localImages {
			parts := strings.Split(imgName, ":")
			repo := parts[0]
			tag := "latest"
			if len(parts) > 1 {
				tag = parts[1]
			}
			fmt.Fprintf(w, "%s\t%s\n", repo, tag)
		}

		if err := w.Flush(); err != nil {
			logrus.Errorf("Failed to flush output: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(imagesCmd)
}
