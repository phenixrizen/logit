package cmd

import (
	"github.com/intwinelabs/logger"
	"github.com/spf13/cobra"

	"github.com/phenixrizen/logit"
)

var expr string
var topics []string

// clientCmd represents the client command
var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "run the client daemon",
	Run: func(cmd *cobra.Command, args []string) {
		runClient()
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().StringVarP(&expr, "expr", "r", "", "regular expression to match on")
	clientCmd.Flags().StringSliceVarP(&topics, "topics", "", []string{}, "topics to match on")
}

func runClient() {
	log := logger.New()
	log.Info("Running in client mode....")
	log.Infof("Looking for logs that match: %s", expr)
	log.Infof("Looking for logs on topics: %v", topics)

	client, errChan, err := logit.NewClient(expr, topics, log)
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
	client.Run()
}
