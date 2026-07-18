package cli

import (
	"fmt"

	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/spf13/cobra"
)

func newPatchCmd() *cobra.Command {
	var in, out string
	c := &cobra.Command{
		Use:   "patch",
		Short: "Patch one APK with ReVanced (defaults + package rename + operator keystore)",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			if err := a.LoadSigning(); err != nil {
				return err
			}
			if in == "" || out == "" {
				return fmt.Errorf("--in and --out are required")
			}
			patches, err := revanced.Patch(revanced.PatchOptions{
				CLIJar:                  a.WS.PatcherJAR(),
				PatchesRVP:              a.WS.PatchesRVP(),
				InputAPK:                in,
				OutputAPK:               out,
				KeystorePath:            a.WS.KeystorePath,
				Blob:                    a.Blob,
				EnableChangePackageName: true,
			})
			if err != nil {
				return err
			}
			fmt.Println("ok", out)
			for _, p := range patches {
				fmt.Println(p)
			}
			return nil
		},
	}
	c.Flags().StringVar(&in, "in", "", "input APK")
	c.Flags().StringVar(&out, "out", "", "output APK")
	return c
}
