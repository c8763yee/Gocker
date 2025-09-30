package cmd

import (
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"gocker/internal/config"
)

var logLevel string

func init() {
	rootCmd.AddCommand()

	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", config.DefaultLogLevel,
		"Set the logging level (\"trace\"|\"debug\"|\"info\"|\"warn\"|\"error\"|\"fatal\"|\"panic\")")
}

var rootCmd = &cobra.Command{
	Use:   "gocker",
	Short: "A simple container runtime written in Go",
	Long:  "Gocker is a simple container runtime written in Go.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			log.Fatalf("Invalid log level: %v", err)
		}
		logrus.SetLevel(level)
		logrus.SetOutput(os.Stdout)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
