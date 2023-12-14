/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

This file is part of KubeBlocks project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package context

import (
	viper "github.com/apecloud/kubeblocks/pkg/viperx"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/cmd/auth/utils"
	"github.com/apecloud/kbcli/pkg/cmd/organization"
	"github.com/apecloud/kbcli/pkg/types"
)

var contextExample = templates.Examples(`
	// Get the environment name currently used by the user.
	kbcli environment current 
	// List all environments created by the current user.
	kbcli environment list
	// Get the description information of environment environment1.
	kbcli environment describe environment1
	// Switch to environment environment2.
	kbcli environment use environment2
`)

const (
	localContext = "local"
)

type Context interface {
	showContext() error
	showContexts() error
	showCurrentContext() error
	showUseContext() error
	showRemoveContext() error
}

type ContextOptions struct {
	ContextName  string
	Context      Context
	OutputFormat string

	genericiooptions.IOStreams
}

func NewContextCmd(streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "environment",
		Aliases: []string{"env"},
		Short: "kbcli environment allows you to manage cloud environment. This command is currently only applicable to cloud," +
			" and currently does not support switching the environment of the local k8s cluster.",
		Example: contextExample,
	}
	cmd.AddCommand(
		newContextListCmd(streams),
		newContextUseCmd(streams),
		newContextCurrentCmd(streams),
		newContextDescribeCmd(streams),
	)
	return cmd
}

func newContextListCmd(streams genericiooptions.IOStreams) *cobra.Command {
	o := &ContextOptions{IOStreams: streams}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all created environments.",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.validate(cmd))
			cmdutil.CheckErr(o.runList())
		},
	}
	return cmd
}

func newContextCurrentCmd(streams genericiooptions.IOStreams) *cobra.Command {
	o := &ContextOptions{IOStreams: streams}

	cmd := &cobra.Command{
		Use:   "current",
		Short: "Get the currently used environment.",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.validate(cmd))
			cmdutil.CheckErr(o.runCurrent())
		},
	}
	return cmd
}

func newContextDescribeCmd(streams genericiooptions.IOStreams) *cobra.Command {
	o := &ContextOptions{IOStreams: streams}

	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Get the description information of a environment.",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.validate(cmd))
			cmdutil.CheckErr(o.runDescribe())
		},
	}

	cmd.Flags().StringVarP(&o.OutputFormat, "output", "o", "human", "Output format (table|yaml|json)")

	return cmd
}

func newContextUseCmd(streams genericiooptions.IOStreams) *cobra.Command {
	o := &ContextOptions{IOStreams: streams}

	cmd := &cobra.Command{
		Use:   "use",
		Short: "Use another environment that you have already created.",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.validate(cmd))
			cmdutil.CheckErr(o.runUse())
		},
	}

	return cmd
}

func (o *ContextOptions) validate(cmd *cobra.Command) error {
	if cmd.Name() == "describe" || cmd.Name() == "use" {
		if o.ContextName == "" {
			return errors.New("environment name is required")
		}
	}

	return nil
}

func (o *ContextOptions) complete(args []string) error {
	if len(args) > 0 {
		o.ContextName = args[0]
	}

	currentOrgAndContext, err := organization.GetCurrentOrgAndContext()
	if err != nil {
		return err
	}

	if o.Context == nil {
		if currentOrgAndContext.CurrentContext != localContext {
			token, err := organization.GetToken()
			if err != nil {
				return err
			}
			o.Context = &CloudContext{
				ContextName:  o.ContextName,
				Token:        token,
				OrgName:      currentOrgAndContext.CurrentOrganization,
				IOStreams:    o.IOStreams,
				APIURL:       viper.GetString(types.CfgKeyOpenAPIServer),
				APIPath:      utils.APIPathV1,
				OutputFormat: o.OutputFormat,
			}
		}
	}

	return nil
}

func (o *ContextOptions) runList() error {
	return o.Context.showContexts()
}

func (o *ContextOptions) runCurrent() error {
	return o.Context.showCurrentContext()
}

func (o *ContextOptions) runDescribe() error {
	return o.Context.showContext()
}

func (o *ContextOptions) runUse() error {
	return o.Context.showUseContext()
}
