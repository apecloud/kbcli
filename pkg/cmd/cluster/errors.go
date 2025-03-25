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

package cluster

import (
	cfgcore "github.com/apecloud/kubeblocks/pkg/configuration/core"
)

var (
	componentNotExistErrMessage        = "cluster[name=%s] does not have component[name=%s]. Please check that --component is spelled correctly."
	missingClusterArgErrMassage        = "cluster name should be specified, using --help."
	missingUpdatedParametersErrMessage = "missing updated parameters, using --help."

	notFoundConfigSpecErrorMessage = "cannot find config spec[%s] for component[name=%s] in the cluster[name=%s]"

	notFoundConfigFileErrorMessage = "cannot find config file[name=%s] in the configspec[name=%s], all configfiles: %v"

	notConfigSchemaPrompt         = "The config template[%s] is not defined in schema and parameter explanation info cannot be generated."
	cue2openAPISchemaFailedPrompt = "The cue schema may not satisfy the conversion constraints of openAPISchema and parameter explanation info cannot be generated."
	restartConfirmPrompt          = "The parameter change incurs a cluster restart, which brings the cluster down for a while. Enter to continue...\n, "
	fullRestartConfirmPrompt      = "The config file[%s] change incurs a cluster restart, which brings the cluster down for a while. Enter to continue...\n, "
	confirmApplyReconfigurePrompt = "Are you sure you want to apply these changes?\n"
)

func makeComponentNotExistErr(clusterName, component string) error {
	return cfgcore.MakeError(componentNotExistErrMessage, clusterName, component)
}

func makeConfigSpecNotExistErr(clusterName, component, configSpec string) error {
	return cfgcore.MakeError(notFoundConfigSpecErrorMessage, configSpec, component, clusterName)
}

func makeNotFoundConfigFileErr(configFile, configSpec string, all []string) error {
	return cfgcore.MakeError(notFoundConfigFileErrorMessage, configFile, configSpec, all)
}

func makeMissingClusterNameErr() error {
	return cfgcore.MakeError(missingClusterArgErrMassage)
}
