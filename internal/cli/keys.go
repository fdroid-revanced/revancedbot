package cli

import (
	"fmt"

	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/spf13/cobra"
)

func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Signing key helpers",
	}
	cmd.AddCommand(newKeysGenerateCmd(), newKeysValidateCmd())
	return cmd
}

func newKeysGenerateCmd() *cobra.Command {
	var alias string
	c := &cobra.Command{
		Use:   "generate",
		Short: "Generate a keystore and print one pasteable signing secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			enc, err := signing.Generate(alias)
			if err != nil {
				return err
			}
			fmt.Println(enc)
			fmt.Fprintln(cmd.ErrOrStderr(), "# Paste the line above into the REVANCEDBOT_SIGNING secret.")
			return nil
		},
	}
	c.Flags().StringVar(&alias, "alias", "revancedbot", "keystore alias")
	return c
}

func newKeysValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate REVANCEDBOT_SIGNING and materialize keystore into the workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			if err := a.LoadSigning(); err != nil {
				return err
			}
			fmt.Println("ok: signing blob valid; keystore at", a.WS.KeystorePath)
			return nil
		},
	}
}
