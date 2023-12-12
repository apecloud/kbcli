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

package flags

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// TmpFlagSet create a tmpFlagSet to handle your custom flags and args.
type TmpFlagSet struct {
	*pflag.FlagSet
	// help flag
	Help bool
}

func NewTmpFlagSet() *TmpFlagSet {
	t := &TmpFlagSet{
		FlagSet: pflag.NewFlagSet("tmp", pflag.ContinueOnError),
		Help:    false,
	}
	t.BoolVarP(&t.Help, "help", "h", false, "") // eat --help and -h
	t.ParseErrorsWhitelist.UnknownFlags = true
	return t
}

// Check checks whether the args and flags of the command is valid.
// and you can specify a customCheckFunc to override the default check function.
func (t *TmpFlagSet) Check(args []string, customCheckFunc func() error) error {
	if customCheckFunc != nil {
		_ = t.Parse(args)
		return customCheckFunc()
	}
	return nil
}

func FlagsToValues(fs *pflag.FlagSet, explicit bool) map[string]pflag.Value {
	values := make(map[string]pflag.Value)
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || (explicit && !f.Changed) {
			return
		}
		values[f.Name] = f.Value
	})
	return values
}

// BuildFlagsWithOpenAPISchema builds the flag from openAPIV3Schema properties.
func BuildFlagsWithOpenAPISchema(cmd *cobra.Command,
	args []string,
	getOpenAPISchema func() (*apiextensionsv1.JSONSchemaProps, error)) error {
	openAPIV3Schema, err := getOpenAPISchema()
	if err != nil {
		return fmt.Errorf("get openAPIV3Schema failed: %s", err.Error())
	}
	if openAPIV3Schema == nil {
		return fmt.Errorf("can not found openAPIV3Schema")
	}
	// Convert apiextensionsv1.JSONSchemaProps to spec.Schema
	schemaData, err := json.Marshal(openAPIV3Schema)
	if err != nil {
		return err
	}
	schema := &spec.Schema{}
	if err = json.Unmarshal(schemaData, schema); err != nil {
		return err
	}
	if err = BuildFlagsBySchema(cmd, schema); err != nil {
		return err
	}
	// Parse dynamic flags
	cmd.DisableFlagParsing = false
	err = cmd.ParseFlags(args)
	if err != nil {
		return err
	}
	helpFlag := cmd.Flags().Lookup("help")
	if helpFlag != nil && helpFlag.Value.String() == "true" {
		return pflag.ErrHelp
	}
	return cmd.ValidateRequiredFlags()
}
