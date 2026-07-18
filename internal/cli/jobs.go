package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newListJobsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-jobs",
		Short: "List packages and preferred versions from ReVanced patches",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			jobs, err := a.ListJobs()
			if err != nil {
				return err
			}
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
					fmt.Printf("%s\n", j.PackageID)
				} else {
					fmt.Printf("%s\t%s\n", j.PackageID, strings.Join(vers, ","))
				}
			}
			return nil
		},
	}
}
