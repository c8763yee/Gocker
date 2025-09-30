// gocker/cmd/ps.go
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"gocker/internal/image"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var psCommand = &cobra.Command{
	Use:   "ps",
	Short: "List all containers",
	Run: func(cmd *cobra.Command, args []string) {
		containers, err := image.ListContainers()
		if err != nil {
			logrus.Fatalf("Failed to list containers: %v", err)
		}

		// 使用 tabwriter 來格式化輸出
		w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
		fmt.Fprint(w, "ID\tNAME\tIMAGE\tCOMMAND\tSTATUS\n")
		for _, c := range containers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				c.ID[:12], // 只顯示前 12 個字元
				c.Name,
				c.Image,
				c.Command,
				c.Status)
		}
		if err := w.Flush(); err != nil {
			logrus.Errorf("Failed to flush output: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(psCommand)
}
