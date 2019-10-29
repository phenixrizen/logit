package cmd

import (
	"github.com/intwinelabs/logger"
	"github.com/spf13/cobra"

	"github.com/phenixrizen/logit"
)

var listenAddr string
var timeout int

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "run the server daemon",
	Run: func(cmd *cobra.Command, args []string) {
		runServer()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&listenAddr, "listenAddr", "l", ":42280", "tcp listen address")
	serverCmd.Flags().IntVarP(&timeout, "timeout", "t", 2, "tcp timeout in minutes")
}

func runServer() {
	log := logger.New()
	log.Info("Running in server mode....")
	log.Infof("Send logs to: %s", listenAddr)

	server, errChan, err := logit.NewServer(listenAddr, timeout, log)
	if err != nil {
		panic(err)
	}

	// print the errors
	go func(log *logger.Logger) {
		for err := range errChan {
			log.Errorf("Error: %s", err)
		}
	}(log)

	// run the server
	server.Run()
}
