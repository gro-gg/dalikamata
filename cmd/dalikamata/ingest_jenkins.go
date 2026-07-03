package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/app"
	"github.com/spf13/cobra"
)

var (
	jenkinsURL          string
	jenkinsUser         string
	jenkinsToken        string
	jenkinsJobs         []string
	jenkinsRepoOverride map[string]string
)

var jenkinsCmd = &cobra.Command{
	Use:   "jenkins",
	Short: "start a Jenkins data ingestion service",
	Long:  `start a Jenkins data ingestion service`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if jenkinsURL == "" {
			return fmt.Errorf("--jenkins-url is required")
		}
		if jenkinsUser == "" {
			return fmt.Errorf("--jenkins-user is required")
		}
		if jenkinsToken == "" {
			return fmt.Errorf("--jenkins-token is required")
		}

		a := app.NewIngestJenkinsApp(slog.Default())
		a.NATSHost = natsURL
		a.NATSPort = natsPort
		a.JenkinsURL = jenkinsURL
		a.JenkinsUser = jenkinsUser
		a.JenkinsToken = jenkinsToken
		a.Jobs = jenkinsJobs
		a.RepoOverrides = jenkinsRepoOverride
		a.CACertsDir = caCertsDir
		a.Interval = jenkinsInterval

		ctx := cmd.Root().Context()
		var wg sync.WaitGroup
		var runErr error

		wg.Go(func() {
			runErr = a.Run(ctx)
		})

		<-ctx.Done()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-time.After(gracePeriod):
			return fmt.Errorf("shutdown grace period expired")
		case <-done:
			return runErr
		}
	},
}

func init() {
	ingestCmd.AddCommand(jenkinsCmd)
	addJenkinsFlags(jenkinsCmd)
}

func addJenkinsFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&jenkinsURL, "jenkins-url", "", "Jenkins base URL (e.g. https://jenkins.example.com)")
	cmd.Flags().StringVar(&jenkinsUser, "jenkins-user", "", "Jenkins username")
	cmd.Flags().StringVar(&jenkinsToken, "jenkins-token", "", "Jenkins API token")
	cmd.Flags().StringSliceVar(&jenkinsJobs, "jenkins-jobs", nil, "full Jenkins job paths to crawl (comma-separated); crawl all if omitted")
	cmd.Flags().StringToStringVar(&jenkinsRepoOverride, "jenkins-repo-override", nil, "manual pipeline→repoID mapping, e.g. my-pipeline=PROJ/my-repo (comma-separated key=value pairs)")
	cmd.Flags().DurationVar(&jenkinsInterval, "jenkins-interval", 5*time.Minute, "how often to re-crawl Jenkins for new builds")
}
