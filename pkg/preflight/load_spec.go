/*
Copyright (C) 2022-2026 ApeCloud Co., Ltd

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

package preflight

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	troubleshoot "github.com/replicatedhq/troubleshoot/pkg/apis/troubleshoot/v1beta2"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/apecloud/kbcli/pkg/cmd/cluster"
)

type preflightKind struct {
	Kind string `json:"kind"`
}

// LoadPreflightSpec loads preflight specs from args.
func LoadPreflightSpec(checkFileList []string, checkYamlData [][]byte) (*troubleshoot.Preflight, string, error) {
	var (
		preflightSpec    *troubleshoot.Preflight
		preflightContent []byte
		preflightName    string
		err              error
	)
	for _, fileName := range checkFileList {
		// support to load yaml from stdin, local file and URI
		if preflightContent, err = cluster.MultipleSourceComponents(fileName, os.Stdin); err != nil {
			return preflightSpec, preflightName, err
		}
		checkYamlData = append(checkYamlData, preflightContent)
	}
	for _, yamlData := range checkYamlData {
		var kind preflightKind
		if err = yaml.Unmarshal(yamlData, &kind); err != nil {
			return preflightSpec, preflightName, errors.Wrapf(err, "failed to parse %s", string(yamlData))
		}
		switch kind.Kind {
		case "Preflight":
			spec := new(troubleshoot.Preflight)
			if err = yaml.Unmarshal(yamlData, spec); err != nil {
				return preflightSpec, preflightName, errors.Wrapf(err, "failed to parse %s", string(yamlData))
			}
			preflightSpec = ConcatPreflightSpec(preflightSpec, spec)
			preflightName = preflightSpec.Name
		default:
			return preflightSpec, preflightName, fmt.Errorf("unsupported preflight kind %q", kind.Kind)
		}
	}
	return preflightSpec, preflightName, nil
}
