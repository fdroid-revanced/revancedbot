package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/spf13/cobra"
	"workspaced/pkg/logging"
	"workspaced/pkg/taskgroup"
)

func newListJobsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-jobs REPO",
		Short: "List packages and preferred versions from cached ReVanced patches",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			ctx := ctxOf(cmd)
			log := logging.GetLogger(ctx)
			return schedule(ctx, "list ReVanced packages", taskgroup.Control, func(ctx context.Context, s *taskgroup.Status) error {
				s.Update("list-versions")
				if err := a.FetchTools(ctx); err != nil {
					return err
				}
				jobs, err := a.ListJobs()
				if err != nil {
					return err
				}
				log.Info("jobs listed", "count", len(jobs))
				// Machine-readable table on stdout.
				for _, line := range formatJobs(jobs) {
					fmt.Println(line)
				}
				return nil
			})
		},
	}
}

func formatJobs(jobs []revanced.Job) []string {
	out := make([]string, 0, len(jobs))
	for _, j := range jobs {
		vers := make([]string, len(j.Versions))
		for i, v := range j.Versions {
			if v == "" {
				vers[i] = "Any"
			} else {
				vers[i] = v
			}
		}
		if len(vers) == 0 {
			out = append(out, j.PackageID)
		} else {
			out = append(out, fmt.Sprintf("%s\t%s", j.PackageID, strings.Join(vers, ",")))
		}
	}
	return out
}
