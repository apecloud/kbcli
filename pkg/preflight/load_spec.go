/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

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
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes/scheme"

	preflightv1beta2 "github.com/apecloud/kubeblocks/externalapis/preflight/v1beta2"

	"github.com/apecloud/kbcli/pkg/preflight/util"
)

// LoadPreflightSpec loads content of preflightSpec and hostPreflightSpec against yamlFiles from args
func LoadPreflightSpec(checkFileList []string, checkYamlData [][]byte) (*preflightv1beta2.Preflight, *preflightv1beta2.HostPreflight, string, error) {
	var (
		preflightSpec     *preflightv1beta2.Preflight
		hostPreflightSpec *preflightv1beta2.HostPreflight
		preflightContent  []byte
		preflightName     string
		err               error
	)
	for _, fileName := range checkFileList {
		// support to load yaml from stdin, local file and URI
		if preflightContent, err = MultipleSourceComponents(fileName, os.Stdin); err != nil {
			return preflightSpec, hostPreflightSpec, preflightName, err
		}
		checkYamlData = append(checkYamlData, preflightContent)
	}
	for _, yamlData := range checkYamlData {
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(yamlData, nil, nil)
		if err != nil {
			return preflightSpec, hostPreflightSpec, preflightName, errors.Wrapf(err, "failed to parse %s", string(yamlData))
		}
		if spec, ok := obj.(*preflightv1beta2.Preflight); ok {
			preflightSpec = ConcatPreflightSpec(preflightSpec, spec)
			preflightName = preflightSpec.Name
		} else if spec, ok := obj.(*preflightv1beta2.HostPreflight); ok {
			hostPreflightSpec = ConcatHostPreflightSpec(hostPreflightSpec, spec)
			preflightName = hostPreflightSpec.Name
		}
	}
	return preflightSpec, hostPreflightSpec, preflightName, nil
}

func init() {
	// register the scheme of troubleshoot API and decode function
	if err := util.AddToScheme(scheme.Scheme); err != nil {
		return
	}
}

// MultipleSourceComponents gets component data from multiple source, such as stdin, URI and local file
func MultipleSourceComponents(fileName string, in io.Reader) ([]byte, error) {
	var data io.Reader
	switch {
	case fileName == "-":
		data = in
	case strings.Index(fileName, "http://") == 0 || strings.Index(fileName, "https://") == 0:
		resp, err := http.Get(fileName)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data = resp.Body
	default:
		f, err := os.Open(fileName)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data = f
	}
	return io.ReadAll(data)
}
