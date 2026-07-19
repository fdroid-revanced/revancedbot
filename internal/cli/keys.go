package cli

import (
	"context"
	"fmt"

	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/spf13/cobra"
	"workspaced/pkg/taskgroup"
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toolscheck.Check(toolscheck.KeysOnly()); err != nil {
				return err
			}
			ctx := ctxOf(cmd)
			return schedule(ctx, "keys-generate", taskgroup.CPU, func(ctx context.Context, s *taskgroup.Status) error {
				defer s.Unit()()
				s.Update("keytool")
				enc, err := signing.Generate(alias)
				if err != nil {
					return err
				}
				afterWait(ctx, func() error {
					fmt.Println(enc)
					fmt.Fprintln(cmd.ErrOrStderr(), "# Paste the line above into the REVANCEDBOT_SIGNING secret.")
					return nil
				})
				return nil
			})
		},
	}
	c.Flags().StringVar(&alias, "alias", "revancedbot", "keystore alias")
	return c
}

func newKeysValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate REPO",
		Short: "Validate REVANCEDBOT_SIGNING and materialize keystore into CACHE",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toolscheck.Check(toolscheck.KeysOnly()); err != nil {
				return err
			}
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			ctx := ctxOf(cmd)
			return schedule(ctx, "keys-validate", taskgroup.CPU, func(ctx context.Context, s *taskgroup.Status) error {
				defer s.Unit()()
				s.Update("validate")
				if err := a.LoadSigning(); err != nil {
					return err
				}
				afterWait(ctx, func() error {
					fmt.Println("ok: signing blob valid; keystore at", a.WS.KeystorePath)
					fmt.Println("cache:", a.WS.Cache)
					return nil
				})
				return nil
			})
		},
	}
}
